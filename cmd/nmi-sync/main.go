package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/open-rails/openrails/config"
	"github.com/open-rails/openrails/internal/app"
	"github.com/open-rails/openrails/internal/db/models"
	"github.com/open-rails/openrails/internal/integrations/nmi"
	"github.com/open-rails/openrails/internal/services"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type nmResponse struct {
	Subscriptions []nmSubscription `xml:"subscription"`
}

type nmSubscription struct {
	SubscriptionID string `xml:"subscription_id"`
}

func main() {
	cmd := &cobra.Command{
		Use:   "nmi-sync",
		Short: "Reconcile local subscriptions against Mobius recurring report",
		RunE:  run,
	}
	cmd.CompletionOptions.DisableDefaultCmd = false

	cmd.Flags().String("config", "./config.yaml", "Path to config file")
	cmd.Flags().String("processor", "mobius", "NMI processor name to reconcile")
	cmd.Flags().Int("result-limit", 100, "Max results per page for NMI query.php")
	cmd.Flags().String("result-order", "reverse", "Result order for NMI query.php")
	cmd.Flags().Int("max-pages", 0, "Max pages to fetch (0 = unlimited)")
	cmd.Flags().Bool("apply", false, "Apply cancellation for local-only subscriptions")
	cmd.Flags().Bool("revoke-access", false, "Revoke entitlements immediately when applying cancellations")

	viper.AutomaticEnv()
	_ = viper.BindPFlags(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, _ []string) error {
	configPath := viper.GetString("config")
	processor := viper.GetString("processor")
	resultLimit := viper.GetInt("result-limit")
	resultOrder := viper.GetString("result-order")
	maxPages := viper.GetInt("max-pages")
	apply := viper.GetBool("apply")
	revokeAccess := viper.GetBool("revoke-access")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	application, err := app.Bootstrap(cfg)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	defer application.Close(context.Background())

	client, err := buildNMIClient(cfg, processor)
	if err != nil {
		return fmt.Errorf("nmi client init failed: %w", err)
	}

	ctx := context.Background()
	remoteIDs, remoteCount, err := fetchRemoteSubscriptions(client, resultLimit, resultOrder, maxPages)
	if err != nil {
		return fmt.Errorf("fetch remote subscriptions failed: %w", err)
	}

	subService := services.NewSubscriptionService(application.Runtime.DB, nil, nil, nil, nil, nil, nil)
	localSubs, err := subService.GetActiveSubscriptionsByProcessor(ctx, strings.TrimSpace(processor))
	if err != nil {
		return fmt.Errorf("fetch local subscriptions failed: %w", err)
	}

	localIDs := make(map[string]struct{}, len(localSubs))
	for _, sub := range localSubs {
		id := strings.TrimSpace(sub.ProcessorSubscriptionID)
		if id != "" {
			localIDs[id] = struct{}{}
		}
	}

	remoteOnly := diffKeys(remoteIDs, localIDs)
	localOnly := diffKeys(localIDs, remoteIDs)

	fmt.Printf("remote subscriptions fetched: %d (unique ids=%d)\n", remoteCount, len(remoteIDs))
	fmt.Printf("local active subscriptions: %d (unique ids=%d)\n", len(localSubs), len(localIDs))

	if len(remoteOnly) == 0 {
		fmt.Println("remote-only subscriptions: none")
	} else {
		fmt.Printf("remote-only subscriptions (%d):\n", len(remoteOnly))
		for _, id := range remoteOnly {
			fmt.Printf("  %s\n", id)
		}
	}

	if len(localOnly) == 0 {
		fmt.Println("local-only subscriptions: none")
	} else {
		fmt.Printf("local-only subscriptions (%d):\n", len(localOnly))
		for _, id := range localOnly {
			fmt.Printf("  %s\n", id)
		}
	}

	if apply && len(localOnly) > 0 {
		if application.Runtime == nil || application.Runtime.SubscriptionLifecycleService == nil {
			return fmt.Errorf("subscription lifecycle service unavailable; cannot apply changes")
		}

		for _, id := range localOnly {
			sub, err := subService.GetByProcessorSubscriptionID(ctx, strings.TrimSpace(processor), "", id)
			if err != nil || sub == nil {
				fmt.Fprintf(os.Stderr, "skip %s: failed to load subscription: %v\n", id, err)
				continue
			}

			cancelFeedback := "Cancelled via nmi-sync reconciliation"
			processorModel := models.Processor(strings.TrimSpace(processor))
			err = application.Runtime.SubscriptionLifecycleService.CancelMembership(ctx, &services.CancelMembershipParams{
				RevokeAccess:            revokeAccess,
				Processor:               &processorModel,
				ProcessorSubscriptionID: &id,
				CancelFeedback:          &cancelFeedback,
				SubscriptionID:          &sub.ID,
				CancelType:              models.CancelTypeMerchant,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to cancel %s: %v\n", id, err)
				continue
			}

			fmt.Printf("cancelled local subscription %s (revoke_access=%t)\n", id, revokeAccess)
		}
	}

	return nil
}

func buildNMIClient(cfg *config.Config, processor string) (*nmi.NMIClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	name := strings.TrimSpace(strings.ToLower(processor))
	if name == "" {
		name = "mobius"
	}

	nmiProcessors := cfg.GetNMIProcessors()
	procConfig, ok := nmiProcessors[name]
	if !ok || procConfig == nil {
		return nil, fmt.Errorf("nmi processor '%s' not configured", name)
	}

	settings := procConfig.ToNMIProviderSettings(name)
	if strings.TrimSpace(settings.SecurityKey) == "" {
		return nil, fmt.Errorf("nmi processor '%s' security key is required", name)
	}

	return nmi.NewClient(name, settings, cfg.IsTestMode())
}

func fetchRemoteSubscriptions(client *nmi.NMIClient, limit int, order string, maxPages int) (map[string]struct{}, int, error) {
	if limit <= 0 {
		limit = 100
	}
	result := make(map[string]struct{})
	totalSeen := 0

	for page := 0; ; page++ {
		if maxPages > 0 && page >= maxPages {
			break
		}

		resp, err := client.QueryRecurringSubscriptions(nmi.RecurringQueryParams{
			ResultLimit: limit,
			PageNumber:  page,
			ResultOrder: order,
		})
		if err != nil {
			return nil, totalSeen, err
		}

		subs, err := parseRecurringSubscriptions(resp)
		if err != nil {
			return nil, totalSeen, err
		}
		if len(subs) == 0 {
			break
		}

		totalSeen += len(subs)
		for _, id := range subs {
			result[id] = struct{}{}
		}

		if len(subs) < limit {
			break
		}
	}

	return result, totalSeen, nil
}

func parseRecurringSubscriptions(payload string) ([]string, error) {
	var resp nmResponse
	if err := xml.Unmarshal([]byte(payload), &resp); err != nil {
		return nil, fmt.Errorf("parse xml: %w", err)
	}

	result := make([]string, 0, len(resp.Subscriptions))
	for _, sub := range resp.Subscriptions {
		id := strings.TrimSpace(sub.SubscriptionID)
		if id != "" {
			result = append(result, id)
		}
	}
	return result, nil
}

func diffKeys(a, b map[string]struct{}) []string {
	out := make([]string, 0)
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
