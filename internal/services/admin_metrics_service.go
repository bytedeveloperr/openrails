package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jonboulle/clockwork"

	"github.com/doujins-org/doujins-billing/config"
)

type AdminMetricsService struct {
	cfg   *config.ClickHouseConfig
	clock clockwork.Clock
}

func NewAdminMetricsService(cfg *config.ClickHouseConfig) *AdminMetricsService {
	return &AdminMetricsService{
		cfg:   cfg,
		clock: clockwork.NewRealClock(),
	}
}

func (s *AdminMetricsService) Clock() clockwork.Clock {
	if s.clock != nil {
		return s.clock
	}
	return clockwork.NewRealClock()
}

func (s *AdminMetricsService) SetClock(clock clockwork.Clock) {
	s.clock = clock
}

type MetricsDateRange struct {
	Start time.Time
	End   time.Time
}

type SummaryResponse struct {
	PeriodStart         time.Time                   `json:"period_start"`
	PeriodEnd           time.Time                   `json:"period_end"`
	Currency            string                      `json:"currency"`
	MRR                 int64                       `json:"mrr"`
	ARR                 int64                       `json:"arr"`
	TotalRevenue        int64                       `json:"total_revenue"`
	SubscriptionRevenue int64                       `json:"subscription_revenue"`
	OneTimeRevenue      int64                       `json:"one_time_revenue"`
	Refunds             int64                       `json:"refunds"`
	Chargebacks         int64                       `json:"chargebacks"`
	NewSubscriptions    int                         `json:"new_subscriptions"`
	Cancellations       CancellationBreakdown       `json:"cancellations"`
	NetNewSubscriptions int                         `json:"net_new_subscriptions"`
	ActiveSubscriptions ActiveSubscriptionBreakdown `json:"active_subscriptions"`
	ARPU                int64                       `json:"arpu"`
	EntitlementGrants   int                         `json:"entitlement_grants"`
	Comparison          *SummaryComparison          `json:"comparison,omitempty"`
	DataFreshAsOf       time.Time                   `json:"data_fresh_as_of"`
}

type CancellationBreakdown struct {
	Total       int `json:"total"`
	Voluntary   int `json:"voluntary"`
	Involuntary int `json:"involuntary"`
}

type ActiveSubscriptionBreakdown struct {
	Active            int `json:"active"`
	PastDue           int `json:"past_due"`
	Pending           int `json:"pending"`
	CancelledInPeriod int `json:"cancelled_in_period"`
}

type SummaryComparison struct {
	PreviousPeriod MetricsDateRange `json:"previous_period"`
	MRRDelta       int64            `json:"mrr_delta"`
	RevenueDelta   int64            `json:"total_revenue_delta"`
	NetNewDelta    int              `json:"net_new_delta"`
}

type RevenueSeriesResponse struct {
	Currency    string                `json:"currency"`
	Granularity string                `json:"granularity"`
	Buckets     []RevenueSeriesBucket `json:"buckets"`
	Totals      RevenueTotals         `json:"totals"`
}

type RevenueSeriesBucket struct {
	PeriodStart         time.Time              `json:"period_start"`
	PeriodEnd           time.Time              `json:"period_end"`
	TotalRevenue        int64                  `json:"total_revenue"`
	SubscriptionRevenue int64                  `json:"subscription_revenue"`
	OneTimeRevenue      int64                  `json:"one_time_revenue"`
	Refunds             int64                  `json:"refunds"`
	Chargebacks         int64                  `json:"chargebacks"`
	Payments            PaymentSeriesBreakdown `json:"payments"`
}

type PaymentSeriesBreakdown struct {
	Count         int   `json:"count"`
	AverageAmount int64 `json:"average_amount"`
}

type RevenueTotals struct {
	TotalRevenue        int64 `json:"total_revenue"`
	SubscriptionRevenue int64 `json:"subscription_revenue"`
	OneTimeRevenue      int64 `json:"one_time_revenue"`
	Refunds             int64 `json:"refunds"`
	Chargebacks         int64 `json:"chargebacks"`
}

type SubscriptionSeriesResponse struct {
	Granularity string                     `json:"granularity"`
	Buckets     []SubscriptionSeriesBucket `json:"buckets"`
	Totals      SubscriptionSeriesTotals   `json:"totals"`
}

type SubscriptionSeriesBucket struct {
	PeriodStart      time.Time                   `json:"period_start"`
	PeriodEnd        time.Time                   `json:"period_end"`
	NewSubscriptions int                         `json:"new_subscriptions"`
	ScheduledStarts  int                         `json:"scheduled_starts"`
	Cancellations    CancellationReasonBreakdown `json:"cancellations"`
	Reactivations    int                         `json:"reactivations"`
	NetChange        int                         `json:"net_change"`
	ActiveCountEnd   int                         `json:"active_count_end"`
}

type SubscriptionSeriesTotals struct {
	NewSubscriptions int `json:"new_subscriptions"`
	Cancellations    int `json:"cancellations"`
	Reactivations    int `json:"reactivations"`
	NetChange        int `json:"net_change"`
}

type CancellationReasonBreakdown struct {
	Voluntary   int `json:"voluntary"`
	Involuntary int `json:"involuntary"`
}

type ProcessorMetricsResponse struct {
	PeriodStart time.Time          `json:"period_start"`
	PeriodEnd   time.Time          `json:"period_end"`
	Processors  []ProcessorMetrics `json:"processors"`
}

type ProcessorMetrics struct {
	Processor string                `json:"processor"`
	Metrics   DailyProcessorMetrics `json:"metrics"`
}

type DailyProcessorMetrics struct {
	Revenue  DailyProcessorRevenue `json:"revenue"`
	Payments DailyProcessorPayment `json:"payments"`

	ActiveSubscriptions int `json:"active_subscriptions"`
	NewSubscriptions    int `json:"new_subscriptions"`
	Cancellations       int `json:"cancellations"`
}

type DailyProcessorRevenue struct {
	Total        int64 `json:"total"`
	Subscription int64 `json:"subscription"`
	OneTime      int64 `json:"one_time"`
	Refunds      int64 `json:"refunds"`
	Chargebacks  int64 `json:"chargebacks"`
}

type DailyProcessorPayment struct {
	Successful int `json:"successful"`
	Failed     int `json:"failed"`
}

type ChurnResponse struct {
	PeriodStart         time.Time              `json:"period_start"`
	PeriodEnd           time.Time              `json:"period_end"`
	MonthlyChurn        []MonthlyChurnPoint    `json:"monthly_churn"`
	CancellationReasons []ReasonCount          `json:"cancellation_reasons"`
	CohortRetention     []CohortRetentionEntry `json:"cohort_retention"`
}

type MonthlyChurnPoint struct {
	Month           string  `json:"month"`
	ChurnRate       float64 `json:"churn_rate"`
	VoluntaryRate   float64 `json:"voluntary_rate"`
	InvoluntaryRate float64 `json:"involuntary_rate"`
	ActiveStart     int     `json:"active_start"`
	ActiveEnd       int     `json:"active_end"`
}

type ReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type CohortRetentionEntry struct {
	Cohort         string                 `json:"cohort"`
	InitialSignups int                    `json:"initial_signups"`
	Retention      []CohortRetentionPoint `json:"retention"`
}

type CohortRetentionPoint struct {
	Month  int     `json:"month"`
	Active int     `json:"active"`
	Rate   float64 `json:"rate"`
}

type periodAggregates struct {
	Currency            string
	CurrencyCount       int64
	SubscriptionRevenue int64
	OneTimeRevenue      int64
	Refunds             int64
	Chargebacks         int64
	NewSubscriptions    int64
	CancellationsVol    int64
	CancellationsInv    int64
	Reactivations       int64
	EntitlementsGranted int64
	ActiveSumObserved   int64
	PastDueSumObserved  int64
	PendingSumObserved  int64
	RowCount            int64
	DayCount            int64
	ActiveLast          int64
	PastDueLast         int64
	PendingLast         int64
	MRR                 int64
	ActiveEnd           int64
	PastDueEnd          int64
	PendingEnd          int64
	DataFreshAsOf       time.Time
}

func bucketStartExpr(granularity string) string {
	switch granularity {
	case "week":
		return "toStartOfWeek(snapshot_date)"
	case "month":
		return "toStartOfMonth(snapshot_date)"
	default:
		return "snapshot_date"
	}
}

func (s *AdminMetricsService) aggregatePeriod(ctx context.Context, startDay, endDay time.Time) (*periodAggregates, error) {
	conn, err := s.openClickHouse()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	aggQuery := `
        SELECT
            anyLast(currency) AS currency,
            uniqExact(currency) AS currency_count,
            sum(subscription_revenue_cents) AS subscription_revenue_cents,
            sum(one_time_revenue_cents) AS one_time_revenue_cents,
            sum(refunds_cents) AS refunds_cents,
            sum(chargebacks_cents) AS chargebacks_cents,
            sum(new_subscriptions) AS new_subscriptions,
            sum(cancellations_user) AS cancellations_user,
            sum(cancellations_merchant) AS cancellations_merchant,
            sum(cancellations_expired) AS cancellations_expired,
            sum(cancellations_chargeback) AS cancellations_chargeback,
            sum(reactivations) AS reactivations,
            sum(entitlements_granted) AS entitlements_granted,
            sum(active_count_end) AS active_sum,
            sum(past_due_count_end) AS past_due_sum,
            sum(pending_count_end) AS pending_sum,
            argMax(active_count_end, snapshot_date) AS active_last,
            argMax(past_due_count_end, snapshot_date) AS past_due_last,
            argMax(pending_count_end, snapshot_date) AS pending_last,
            count() AS row_count
        FROM daily_metrics
        WHERE snapshot_date >= ? AND snapshot_date <= ?`

	var agg struct {
		Currency              string `ch:"currency"`
		CurrencyCount         int64  `ch:"currency_count"`
		SubscriptionRevenue   int64  `ch:"subscription_revenue_cents"`
		OneTimeRevenue        int64  `ch:"one_time_revenue_cents"`
		Refunds               int64  `ch:"refunds_cents"`
		Chargebacks           int64  `ch:"chargebacks_cents"`
		NewSubscriptions      int64  `ch:"new_subscriptions"`
		CancellationsUser     int64  `ch:"cancellations_user"`
		CancellationsMerchant int64  `ch:"cancellations_merchant"`
		CancellationsExpired  int64  `ch:"cancellations_expired"`
		CancellationsCb       int64  `ch:"cancellations_chargeback"`
		Reactivations         int64  `ch:"reactivations"`
		EntitlementsGranted   int64  `ch:"entitlements_granted"`
		ActiveSum             int64  `ch:"active_sum"`
		PastDueSum            int64  `ch:"past_due_sum"`
		PendingSum            int64  `ch:"pending_sum"`
		ActiveLast            int64  `ch:"active_last"`
		PastDueLast           int64  `ch:"past_due_last"`
		PendingLast           int64  `ch:"pending_last"`
		RowCount              int64  `ch:"row_count"`
	}

	if err := conn.QueryRow(ctx, aggQuery, startDay, endDay).ScanStruct(&agg); err != nil {
		return nil, err
	}
	if agg.RowCount == 0 {
		return nil, nil
	}

	latestQuery := `
        SELECT currency, mrr_cents, active_count_end, past_due_count_end, pending_count_end, created_at
        FROM daily_metrics
        WHERE snapshot_date >= ? AND snapshot_date <= ?
        ORDER BY snapshot_date DESC
        LIMIT 1`

	var latest struct {
		Currency    string    `ch:"currency"`
		MRR         int64     `ch:"mrr_cents"`
		ActiveEnd   int64     `ch:"active_count_end"`
		PastDueEnd  int64     `ch:"past_due_count_end"`
		PendingEnd  int64     `ch:"pending_count_end"`
		DataFreshAs time.Time `ch:"created_at"`
	}

	if err := conn.QueryRow(ctx, latestQuery, startDay, endDay).ScanStruct(&latest); err != nil {
		return nil, err
	}

	totalDays := int64(endDay.Sub(startDay).Hours()/24) + 1
	missingDays := totalDays - agg.RowCount
	if missingDays < 0 {
		missingDays = 0
	}

	volCancels := agg.CancellationsUser + agg.CancellationsMerchant
	invCancels := agg.CancellationsExpired + agg.CancellationsCb

	currency := agg.Currency
	if latest.Currency != "" {
		currency = latest.Currency
	}
	if currency == "" {
		currency = "usd"
	}

	return &periodAggregates{
		Currency:            currency,
		CurrencyCount:       agg.CurrencyCount,
		SubscriptionRevenue: agg.SubscriptionRevenue,
		OneTimeRevenue:      agg.OneTimeRevenue,
		Refunds:             agg.Refunds,
		Chargebacks:         agg.Chargebacks,
		NewSubscriptions:    agg.NewSubscriptions,
		CancellationsVol:    volCancels,
		CancellationsInv:    invCancels,
		Reactivations:       agg.Reactivations,
		EntitlementsGranted: agg.EntitlementsGranted,
		ActiveSumObserved:   agg.ActiveSum,
		PastDueSumObserved:  agg.PastDueSum,
		PendingSumObserved:  agg.PendingSum,
		RowCount:            agg.RowCount,
		DayCount:            totalDays,
		ActiveLast:          agg.ActiveLast,
		PastDueLast:         agg.PastDueLast,
		PendingLast:         agg.PendingLast,
		MRR:                 latest.MRR,
		ActiveEnd:           latest.ActiveEnd,
		PastDueEnd:          latest.PastDueEnd,
		PendingEnd:          latest.PendingEnd,
		DataFreshAsOf:       latest.DataFreshAs,
	}, nil
}

// GetSummary returns aggregate metrics for the requested range using ClickHouse daily_metrics.
func (s *AdminMetricsService) GetSummary(ctx context.Context, rng MetricsDateRange) (*SummaryResponse, error) {
	startDay := truncateToDay(rng.Start)
	endDay := truncateToDay(rng.End.Add(-time.Nanosecond))

	current, err := s.aggregatePeriod(ctx, startDay, endDay)
	if err != nil {
		return nil, err
	}
	if current == nil {
		start := truncateToDay(rng.Start)
		end := truncateToDay(rng.End.Add(-time.Nanosecond)).Add(24 * time.Hour)
		return &SummaryResponse{
			PeriodStart:         start,
			PeriodEnd:           end,
			Currency:            "usd",
			Cancellations:       CancellationBreakdown{},
			ActiveSubscriptions: ActiveSubscriptionBreakdown{},
			DataFreshAsOf:       s.Clock().Now().UTC(),
		}, nil
	}
	if current.CurrencyCount > 1 {
		return nil, fmt.Errorf("mixed currencies detected in range; aggregation requires single currency")
	}

	totalRevenue := current.SubscriptionRevenue + current.OneTimeRevenue
	subRevenue := current.SubscriptionRevenue
	oneTimeRevenue := current.OneTimeRevenue
	refunds := current.Refunds
	chargebacks := current.Chargebacks
	newSubs := int(current.NewSubscriptions)
	cancelsVol := int(current.CancellationsVol)
	cancelsInv := int(current.CancellationsInv)
	reactivations := int(current.Reactivations)
	missingDays := current.DayCount - current.RowCount
	if missingDays < 0 {
		missingDays = 0
	}
	activeSumFilled := current.ActiveSumObserved + missingDays*current.ActiveLast

	activeBreakdown := ActiveSubscriptionBreakdown{
		Active:            int(current.ActiveEnd),
		PastDue:           int(current.PastDueEnd),
		Pending:           int(current.PendingEnd),
		CancelledInPeriod: cancelsVol + cancelsInv,
	}
	mrr := current.MRR
	arr := mrr * 12

	var arpu int64
	if current.DayCount > 0 {
		avgActive := activeSumFilled / current.DayCount
		if avgActive > 0 {
			arpu = totalRevenue / avgActive
		}
	}

	netNew := newSubs - (cancelsVol + cancelsInv) + reactivations

	resp := &SummaryResponse{
		PeriodStart:         startDay,
		PeriodEnd:           endDay.Add(24 * time.Hour),
		Currency:            current.Currency,
		MRR:                 mrr,
		ARR:                 arr,
		TotalRevenue:        totalRevenue,
		SubscriptionRevenue: subRevenue,
		OneTimeRevenue:      oneTimeRevenue,
		Refunds:             refunds,
		Chargebacks:         chargebacks,
		NewSubscriptions:    newSubs,
		Cancellations: CancellationBreakdown{
			Total:       cancelsVol + cancelsInv,
			Voluntary:   cancelsVol,
			Involuntary: cancelsInv,
		},
		NetNewSubscriptions: netNew,
		ActiveSubscriptions: activeBreakdown,
		ARPU:                arpu,
		EntitlementGrants:   int(current.EntitlementsGranted),
		DataFreshAsOf:       current.DataFreshAsOf,
	}

	// Previous period comparison
	prevLength := endDay.Add(24 * time.Hour).Sub(startDay)
	prevStart := startDay.Add(-prevLength)
	prevEnd := startDay.Add(-time.Nanosecond)
	prev, err := s.aggregatePeriod(ctx, prevStart, prevEnd)
	if err == nil && prev != nil {
		prevTotals := aggregateResult{
			MRR:          prev.MRR,
			TotalRevenue: prev.SubscriptionRevenue + prev.OneTimeRevenue,
			NetNew:       int(prev.NewSubscriptions - prev.CancellationsVol - prev.CancellationsInv + prev.Reactivations),
		}
		resp.Comparison = &SummaryComparison{
			PreviousPeriod: MetricsDateRange{Start: prevStart, End: prevEnd.Add(24 * time.Hour)},
			MRRDelta:       resp.MRR - prevTotals.MRR,
			RevenueDelta:   resp.TotalRevenue - prevTotals.TotalRevenue,
			NetNewDelta:    resp.NetNewSubscriptions - prevTotals.NetNew,
		}
	}

	return resp, nil
}

type aggregateResult struct {
	MRR          int64
	TotalRevenue int64
	NetNew       int
}

func (s *AdminMetricsService) GetRevenueSeries(ctx context.Context, rng MetricsDateRange, granularity string) (*RevenueSeriesResponse, error) {
	if granularity == "" {
		granularity = "day"
	}
	startDay := truncateToDay(rng.Start)
	endDay := truncateToDay(rng.End.Add(-time.Nanosecond))

	buckets, err := s.queryRevenueBuckets(ctx, startDay, endDay, granularity)
	if err != nil {
		return nil, err
	}

	resp := &RevenueSeriesResponse{Currency: "usd", Granularity: granularity}
	if len(buckets) > 0 {
		resp.Currency = buckets[len(buckets)-1].Currency
	}

	for _, r := range buckets {
		next := advance(r.BucketStart, granularity)
		if next.After(endDay.Add(24 * time.Hour)) {
			next = endDay.Add(24 * time.Hour)
		}
		bucket := RevenueSeriesBucket{
			PeriodStart:         r.BucketStart,
			PeriodEnd:           next,
			TotalRevenue:        r.TotalRevenue,
			SubscriptionRevenue: r.SubscriptionRevenue,
			OneTimeRevenue:      r.OneTimeRevenue,
			Refunds:             r.Refunds,
			Chargebacks:         r.Chargebacks,
		}
		if r.PaymentsSuccessful > 0 {
			bucket.Payments.Count = int(r.PaymentsSuccessful)
			bucket.Payments.AverageAmount = r.TotalRevenue / int64(r.PaymentsSuccessful)
		}
		resp.Buckets = append(resp.Buckets, bucket)
	}

	for _, b := range resp.Buckets {
		resp.Totals.TotalRevenue += b.TotalRevenue
		resp.Totals.SubscriptionRevenue += b.SubscriptionRevenue
		resp.Totals.OneTimeRevenue += b.OneTimeRevenue
		resp.Totals.Refunds += b.Refunds
		resp.Totals.Chargebacks += b.Chargebacks
	}
	return resp, nil
}

func (s *AdminMetricsService) GetSubscriptionSeries(ctx context.Context, rng MetricsDateRange, granularity string) (*SubscriptionSeriesResponse, error) {
	if granularity == "" {
		granularity = "day"
	}
	startDay := truncateToDay(rng.Start)
	endDay := truncateToDay(rng.End.Add(-time.Nanosecond))

	buckets, err := s.querySubscriptionBuckets(ctx, startDay, endDay, granularity)
	if err != nil {
		return nil, err
	}

	resp := &SubscriptionSeriesResponse{Granularity: granularity}
	for _, r := range buckets {
		next := advance(r.BucketStart, granularity)
		if next.After(endDay.Add(24 * time.Hour)) {
			next = endDay.Add(24 * time.Hour)
		}
		bucket := SubscriptionSeriesBucket{
			PeriodStart:      r.BucketStart,
			PeriodEnd:        next,
			NewSubscriptions: int(r.NewSubscriptions),
			ScheduledStarts:  int(r.ScheduledStarts),
			Cancellations: CancellationReasonBreakdown{
				Voluntary:   int(r.CancellationsUser + r.CancellationsMerchant),
				Involuntary: int(r.CancellationsExpired + r.CancellationsChargeback),
			},
			Reactivations:  int(r.Reactivations),
			ActiveCountEnd: int(r.ActiveCountEnd),
		}
		bucket.NetChange = bucket.NewSubscriptions - (bucket.Cancellations.Voluntary + bucket.Cancellations.Involuntary) + bucket.Reactivations
		resp.Buckets = append(resp.Buckets, bucket)
	}

	for _, b := range resp.Buckets {
		resp.Totals.NewSubscriptions += b.NewSubscriptions
		resp.Totals.Cancellations += b.Cancellations.Voluntary + b.Cancellations.Involuntary
		resp.Totals.Reactivations += b.Reactivations
		resp.Totals.NetChange += b.NetChange
	}
	return resp, nil
}

func (s *AdminMetricsService) GetProcessorMetrics(ctx context.Context, rng MetricsDateRange) (*ProcessorMetricsResponse, error) {
	startDay := truncateToDay(rng.Start)
	endDay := truncateToDay(rng.End.Add(-time.Nanosecond))

	aggRows, err := s.queryProcessorAggregates(ctx, startDay, endDay)
	if err != nil {
		return nil, err
	}

	resp := &ProcessorMetricsResponse{
		PeriodStart: startDay,
		PeriodEnd:   endDay.Add(24 * time.Hour),
	}
	for _, r := range aggRows {
		resp.Processors = append(resp.Processors, ProcessorMetrics{
			Processor: r.Processor,
			Metrics: DailyProcessorMetrics{
				Revenue: DailyProcessorRevenue{
					Total:        r.RevenueTotal,
					Subscription: r.RevenueSubscription,
					OneTime:      r.RevenueOneTime,
					Refunds:      r.RevenueRefunds,
					Chargebacks:  r.RevenueChargebacks,
				},
				Payments: DailyProcessorPayment{
					Successful: int(r.PaymentsSuccessful),
					Failed:     int(r.PaymentsFailed),
				},
				ActiveSubscriptions: int(r.ActiveSubscriptions),
				NewSubscriptions:    int(r.NewSubscriptions),
				Cancellations:       int(r.Cancellations),
			},
		})
	}
	sort.Slice(resp.Processors, func(i, j int) bool {
		return resp.Processors[i].Processor < resp.Processors[j].Processor
	})
	return resp, nil
}

func (s *AdminMetricsService) GetChurn(ctx context.Context, rng MetricsDateRange) (*ChurnResponse, error) {
	startMonth := firstOfMonth(rng.Start)
	endMonthStart := firstOfMonth(rng.End)
	endBoundary := endMonthStart.AddDate(0, 1, 0).Add(-time.Nanosecond)

	monthly, err := s.queryChurnBuckets(ctx, startMonth, endBoundary)
	if err != nil {
		return nil, err
	}

	resp := &ChurnResponse{
		PeriodStart: startMonth,
		PeriodEnd:   endMonthStart.AddDate(0, 1, 0),
	}

	prevActive := 0
	reasonCounts := map[string]int{"user": 0, "merchant": 0, "expired": 0, "chargeback": 0}

	for _, m := range monthly {
		totalCancels := int(m.CancellationsUser + m.CancellationsMerchant + m.CancellationsExpired + m.CancellationsChargeback)

		activeStart := prevActive
		if activeStart == 0 {
			activeStart = int(m.ActiveEnd) + totalCancels
		}

		var churnRate, volRate, invRate float64
		if activeStart > 0 {
			churnRate = float64(totalCancels) / float64(activeStart)
			volRate = float64(m.CancellationsUser+m.CancellationsMerchant) / float64(activeStart)
			invRate = float64(m.CancellationsExpired+m.CancellationsChargeback) / float64(activeStart)
		}

		resp.MonthlyChurn = append(resp.MonthlyChurn, MonthlyChurnPoint{
			Month:           m.MonthStart.Format("2006-01"),
			ChurnRate:       churnRate,
			VoluntaryRate:   volRate,
			InvoluntaryRate: invRate,
			ActiveStart:     activeStart,
			ActiveEnd:       int(m.ActiveEnd),
		})

		reasonCounts["user"] += int(m.CancellationsUser)
		reasonCounts["merchant"] += int(m.CancellationsMerchant)
		reasonCounts["expired"] += int(m.CancellationsExpired)
		reasonCounts["chargeback"] += int(m.CancellationsChargeback)

		prevActive = int(m.ActiveEnd)
	}

	for reason, count := range reasonCounts {
		if count == 0 {
			continue
		}
		resp.CancellationReasons = append(resp.CancellationReasons, ReasonCount{
			Reason: reason,
			Count:  count,
		})
	}

	// Cohort retention: not implemented (no cohort table); return empty slice
	resp.CohortRetention = []CohortRetentionEntry{}
	return resp, nil
}

type revenueBucketRow struct {
	BucketStart         time.Time `ch:"bucket_start"`
	Currency            string    `ch:"currency"`
	SubscriptionRevenue int64     `ch:"subscription_revenue_cents"`
	OneTimeRevenue      int64     `ch:"one_time_revenue_cents"`
	Refunds             int64     `ch:"refunds_cents"`
	Chargebacks         int64     `ch:"chargebacks_cents"`
	TotalRevenue        int64     `ch:"total_revenue_cents"`
	PaymentsSuccessful  uint64    `ch:"payments_successful"`
}

func (s *AdminMetricsService) queryRevenueBuckets(ctx context.Context, startDay, endDay time.Time, granularity string) ([]revenueBucketRow, error) {
	conn, err := s.openClickHouse()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	bucketExpr := bucketStartExpr(granularity)
	query := fmt.Sprintf(`
        SELECT
            %[1]s AS bucket_start,
            anyLast(currency) AS currency,
            sum(subscription_revenue_cents) AS subscription_revenue_cents,
            sum(one_time_revenue_cents) AS one_time_revenue_cents,
            sum(refunds_cents) AS refunds_cents,
            sum(chargebacks_cents) AS chargebacks_cents,
            sum(total_revenue_cents) AS total_revenue_cents,
            sum(payments_successful) AS payments_successful
        FROM daily_metrics
        WHERE snapshot_date >= ? AND snapshot_date <= ?
        GROUP BY bucket_start
        ORDER BY bucket_start`, bucketExpr)

	rows, err := conn.Query(ctx, query, startDay, endDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []revenueBucketRow
	for rows.Next() {
		var r revenueBucketRow
		if err := rows.ScanStruct(&r); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

type subscriptionBucketRow struct {
	BucketStart             time.Time `ch:"bucket_start"`
	NewSubscriptions        int64     `ch:"new_subscriptions"`
	ScheduledStarts         int64     `ch:"scheduled_starts"`
	CancellationsUser       int64     `ch:"cancellations_user"`
	CancellationsMerchant   int64     `ch:"cancellations_merchant"`
	CancellationsExpired    int64     `ch:"cancellations_expired"`
	CancellationsChargeback int64     `ch:"cancellations_chargeback"`
	Reactivations           int64     `ch:"reactivations"`
	ActiveCountEnd          int64     `ch:"active_count_end"`
}

func (s *AdminMetricsService) querySubscriptionBuckets(ctx context.Context, startDay, endDay time.Time, granularity string) ([]subscriptionBucketRow, error) {
	conn, err := s.openClickHouse()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	bucketExpr := bucketStartExpr(granularity)
	query := fmt.Sprintf(`
        SELECT
            %[1]s AS bucket_start,
            sum(new_subscriptions) AS new_subscriptions,
            sum(scheduled_starts) AS scheduled_starts,
            sum(cancellations_user) AS cancellations_user,
            sum(cancellations_merchant) AS cancellations_merchant,
            sum(cancellations_expired) AS cancellations_expired,
            sum(cancellations_chargeback) AS cancellations_chargeback,
            sum(reactivations) AS reactivations,
            argMax(active_count_end, snapshot_date) AS active_count_end
        FROM daily_metrics
        WHERE snapshot_date >= ? AND snapshot_date <= ?
        GROUP BY bucket_start
        ORDER BY bucket_start`, bucketExpr)

	rows, err := conn.Query(ctx, query, startDay, endDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []subscriptionBucketRow
	for rows.Next() {
		var r subscriptionBucketRow
		if err := rows.ScanStruct(&r); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

type processorAggRow struct {
	Processor           string `ch:"processor"`
	ActiveSubscriptions int64  `ch:"active_subscriptions"`
	NewSubscriptions    int64  `ch:"new_subscriptions"`
	Cancellations       int64  `ch:"cancellations"`
	RevenueTotal        int64  `ch:"revenue_total_cents"`
	RevenueSubscription int64  `ch:"revenue_subscription_cents"`
	RevenueOneTime      int64  `ch:"revenue_one_time_cents"`
	RevenueRefunds      int64  `ch:"revenue_refunds_cents"`
	RevenueChargebacks  int64  `ch:"revenue_chargebacks_cents"`
	PaymentsSuccessful  int64  `ch:"payments_successful"`
	PaymentsFailed      int64  `ch:"payments_failed"`
}

func (s *AdminMetricsService) queryProcessorAggregates(ctx context.Context, startDay, endDay time.Time) ([]processorAggRow, error) {
	conn, err := s.openClickHouse()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	query := `
        SELECT
            proc.1 AS processor,
            sum(proc.2) AS active_subscriptions,
            sum(proc.3) AS new_subscriptions,
            sum(proc.4) AS cancellations,
            sum(proc.5) AS revenue_total_cents,
            sum(proc.6) AS revenue_subscription_cents,
            sum(proc.7) AS revenue_one_time_cents,
            sum(proc.8) AS revenue_refunds_cents,
            sum(proc.9) AS revenue_chargebacks_cents,
            sum(proc.10) AS payments_successful,
            sum(proc.11) AS payments_failed
        FROM (
            SELECT arrayJoin(arrayZip(
                processor.name,
                processor.active_subscriptions,
                processor.new_subscriptions,
                processor.cancellations,
                processor.revenue_total_cents,
                processor.revenue_subscription_cents,
                processor.revenue_one_time_cents,
                processor.revenue_refunds_cents,
                processor.revenue_chargebacks_cents,
                processor.payments_successful,
                processor.payments_failed
            )) AS proc
            FROM daily_metrics
            WHERE snapshot_date >= ? AND snapshot_date <= ?
        )
        GROUP BY processor
        ORDER BY processor`

	rows, err := conn.Query(ctx, query, startDay, endDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []processorAggRow
	for rows.Next() {
		var r processorAggRow
		if err := rows.ScanStruct(&r); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

type churnBucketRow struct {
	MonthStart              time.Time `ch:"month_start"`
	CancellationsUser       int64     `ch:"cancellations_user"`
	CancellationsMerchant   int64     `ch:"cancellations_merchant"`
	CancellationsExpired    int64     `ch:"cancellations_expired"`
	CancellationsChargeback int64     `ch:"cancellations_chargeback"`
	ActiveEnd               int64     `ch:"active_count_end"`
}

func (s *AdminMetricsService) queryChurnBuckets(ctx context.Context, startMonth, endDay time.Time) ([]churnBucketRow, error) {
	conn, err := s.openClickHouse()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	query := `
        SELECT
            toStartOfMonth(snapshot_date) AS month_start,
            sum(cancellations_user) AS cancellations_user,
            sum(cancellations_merchant) AS cancellations_merchant,
            sum(cancellations_expired) AS cancellations_expired,
            sum(cancellations_chargeback) AS cancellations_chargeback,
            argMax(active_count_end, snapshot_date) AS active_count_end
        FROM daily_metrics
        WHERE snapshot_date >= ? AND snapshot_date <= ?
        GROUP BY month_start
        ORDER BY month_start`

	rows, err := conn.Query(ctx, query, startMonth, endDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []churnBucketRow
	for rows.Next() {
		var r churnBucketRow
		if err := rows.ScanStruct(&r); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func advance(day time.Time, granularity string) time.Time {
	switch granularity {
	case "week":
		return day.AddDate(0, 0, 7)
	case "month":
		return time.Date(day.Year(), day.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	default:
		return day.AddDate(0, 0, 1)
	}
}

func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (s *AdminMetricsService) openClickHouse() (driver.Conn, error) {
	if s.cfg == nil || s.cfg.ClientAddr == "" {
		return nil, fmt.Errorf("clickhouse not configured")
	}
	opts := &clickhouse.Options{
		Addr: []string{s.cfg.ClientAddr},
		Auth: clickhouse.Auth{
			Database: s.cfg.Database,
			Username: s.cfg.Username,
			Password: s.cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout: 10 * time.Second,
	}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping clickhouse: %w", err)
	}
	return conn, nil
}
