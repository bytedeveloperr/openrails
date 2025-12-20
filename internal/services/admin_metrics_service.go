package services

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/jonboulle/clockwork"
	"github.com/uptrace/bun"
)

type AdminMetricsService struct {
	db    *db.DB
	repo  *repo.BillingAnalyticsRepo
	clock clockwork.Clock
}

func NewAdminMetricsService(database *db.DB) *AdminMetricsService {
	return &AdminMetricsService{
		db:    database,
		repo:  repo.NewBillingAnalyticsRepo(database),
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
	Processor string                       `json:"processor"`
	Metrics   models.DailyProcessorMetrics `json:"metrics"`
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

// loadSnapshots ensures snapshots for every day in [start,end) exist and returns them.
func (s *AdminMetricsService) loadSnapshots(ctx context.Context, start, end time.Time) (map[time.Time]*models.DailyMetricsSnapshot, error) {
	startDay := truncateToDay(start)
	endDay := truncateToDay(end.Add(-time.Nanosecond))
	if endDay.Before(startDay) {
		endDay = startDay
	}

	existing, err := s.repo.GetSnapshotsBetween(ctx, startDay, endDay)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	cache := make(map[time.Time]*models.DailyMetricsSnapshot, len(existing))
	for i := range existing {
		day := truncateToDay(existing[i].SnapshotDate)
		entry := existing[i]
		cache[day] = &entry
	}

	now := s.Clock().Now().UTC()
	today := truncateToDay(now)

	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		snap, ok := cache[day]
		needsRefresh := !ok
		if ok && !needsRefresh {
			if !day.Before(today) && now.Sub(snap.UpdatedAt) > 5*time.Minute {
				needsRefresh = true
			}
		}
		if !needsRefresh {
			continue
		}
		calculated, calcErr := s.calculateSnapshot(ctx, day)
		if calcErr != nil {
			return nil, calcErr
		}
		if err := s.repo.UpsertSnapshot(ctx, calculated); err != nil {
			return nil, err
		}
		cache[day] = calculated
	}

	return cache, nil
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *AdminMetricsService) calculateSnapshot(ctx context.Context, day time.Time) (*models.DailyMetricsSnapshot, error) {
	start := truncateToDay(day)
	end := start.Add(24 * time.Hour)
	bunDB := s.db.GetDB()

	snapshot := &models.DailyMetricsSnapshot{
		SnapshotDate:        start,
		Currency:            "usd",
		ProcessorBreakdowns: make(map[string]models.DailyProcessorMetrics),
		UpdatedAt:           s.Clock().Now().UTC(),
	}

	if err := s.populateSnapshotRevenue(ctx, bunDB, snapshot, start, end); err != nil {
		return nil, err
	}
	if err := s.populateSnapshotSubscriptions(ctx, bunDB, snapshot, start, end); err != nil {
		return nil, err
	}
	if err := s.populateSnapshotProcessors(ctx, bunDB, snapshot, start, end); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func (s *AdminMetricsService) populateSnapshotRevenue(
	ctx context.Context,
	db bun.IDB,
	snapshot *models.DailyMetricsSnapshot,
	start, end time.Time,
) error {
	var subscriptionRevenue int64
	if err := db.NewSelect().
		Model((*models.Payment)(nil)).
		ColumnExpr("COALESCE(SUM(purch.amount),0)").
		Where("purch.amount > 0").
		Where("purch.subscription_id IS NOT NULL").
		Where("purchased_at >= ? AND purchased_at < ?", start, end).
		Scan(ctx, &subscriptionRevenue); err != nil && err != sql.ErrNoRows {
		return err
	}

	var oneTimeRevenue int64
	if err := db.NewSelect().
		Model((*models.Payment)(nil)).
		ColumnExpr("COALESCE(SUM(purch.amount),0)").
		Where("purch.amount > 0").
		Where("purch.subscription_id IS NULL").
		Where("purchased_at >= ? AND purchased_at < ?", start, end).
		Scan(ctx, &oneTimeRevenue); err != nil && err != sql.ErrNoRows {
		return err
	}

	var refunds int64
	if err := db.NewSelect().
		Model((*models.Payment)(nil)).
		ColumnExpr("COALESCE(SUM(ABS(purch.amount)),0)").
		Where("purch.amount < 0").
		Where("purch.refunded_payment_id IS NOT NULL").
		Where("purchased_at >= ? AND purchased_at < ?", start, end).
		Scan(ctx, &refunds); err != nil && err != sql.ErrNoRows {
		return err
	}

	var chargebacks int64
	if err := db.NewSelect().
		Model((*models.Payment)(nil)).
		ColumnExpr("COALESCE(SUM(ABS(purch.amount)),0)").
		Where("purch.amount < 0").
		Where("purch.refunded_payment_id IS NULL").
		Where("purchased_at >= ? AND purchased_at < ?", start, end).
		Scan(ctx, &chargebacks); err != nil && err != sql.ErrNoRows {
		return err
	}

	snapshot.SubscriptionRevenue = subscriptionRevenue
	snapshot.OneTimeRevenue = oneTimeRevenue
	snapshot.RefundsCents = refunds
	snapshot.ChargebacksCents = chargebacks

	var entitlementGrants int
	if err := db.NewSelect().
		Model((*models.Entitlement)(nil)).
		ColumnExpr("COUNT(*)").
		Where("ent.source_type IN (?)", bun.In([]models.EntitlementSourceType{
			models.EntitlementSourceAdmin,
			models.EntitlementSourceGrace,
		})).
		Where("ent.created_at >= ? AND ent.created_at < ?", start, end).
		Scan(ctx, &entitlementGrants); err != nil && err != sql.ErrNoRows {
		return err
	}
	snapshot.EntitlementsGranted = entitlementGrants

	return nil
}

func (s *AdminMetricsService) GetSummary(ctx context.Context, rng MetricsDateRange) (*SummaryResponse, error) {
	startDay := truncateToDay(rng.Start)
	endDay := truncateToDay(rng.End.Add(-time.Nanosecond))
	if endDay.Before(startDay) {
		endDay = startDay
	}

	cache, err := s.loadSnapshots(ctx, startDay, endDay)
	if err != nil {
		return nil, err
	}
	list := snapshotsInRange(cache, startDay, endDay)
	if len(list) == 0 {
		return &SummaryResponse{
			PeriodStart:         startDay,
			PeriodEnd:           endDay.Add(24 * time.Hour),
			Currency:            "usd",
			Cancellations:       CancellationBreakdown{},
			ActiveSubscriptions: ActiveSubscriptionBreakdown{},
			DataFreshAsOf:       s.Clock().Now().UTC(),
		}, nil
	}

	var totalRevenue, subRevenue, oneTimeRevenue, refunds, chargebacks int64
	var newSubs, volCancels, invCancels, reactivations int
	var latest *models.DailyMetricsSnapshot
	for _, snap := range list {
		totalRevenue += snap.SubscriptionRevenue + snap.OneTimeRevenue
		subRevenue += snap.SubscriptionRevenue
		oneTimeRevenue += snap.OneTimeRevenue
		refunds += snap.RefundsCents
		chargebacks += snap.ChargebacksCents
		newSubs += snap.NewSubscriptions
		volCancels += snap.CancellationsVol
		invCancels += snap.CancellationsInv
		reactivations += snap.Reactivations
		latest = snap
	}

	activeBreakdown := ActiveSubscriptionBreakdown{}
	dataFresh := s.Clock().Now().UTC()
	mrr := int64(0)
	if latest != nil {
		mrr = latest.MRRCents
		activeBreakdown.Active = latest.ActiveCountEnd
		activeBreakdown.PastDue = latest.PastDueCountEnd
		activeBreakdown.Pending = latest.PendingCountEnd
		activeBreakdown.CancelledInPeriod = volCancels + invCancels
		dataFresh = latest.UpdatedAt
	}

	arr := mrr * 12

	var arpu int64
	if activeBreakdown.Active > 0 {
		arpu = totalRevenue / int64(activeBreakdown.Active)
	}

	netNew := newSubs - (volCancels + invCancels) + reactivations

	response := &SummaryResponse{
		PeriodStart:         startDay,
		PeriodEnd:           endDay.Add(24 * time.Hour),
		Currency:            "usd",
		MRR:                 mrr,
		ARR:                 arr,
		TotalRevenue:        totalRevenue,
		SubscriptionRevenue: subRevenue,
		OneTimeRevenue:      oneTimeRevenue,
		Refunds:             refunds,
		Chargebacks:         chargebacks,
		NewSubscriptions:    newSubs,
		Cancellations: CancellationBreakdown{
			Total:       volCancels + invCancels,
			Voluntary:   volCancels,
			Involuntary: invCancels,
		},
		NetNewSubscriptions: netNew,
		ActiveSubscriptions: activeBreakdown,
		ARPU:                arpu,
		EntitlementGrants:   sumEntitlements(list),
		DataFreshAsOf:       dataFresh,
	}

	prevLength := endDay.Add(24 * time.Hour).Sub(startDay)
	prevStart := startDay.Add(-prevLength)
	prevEnd := startDay.Add(-time.Nanosecond)
	prevCache, err := s.loadSnapshots(ctx, prevStart, prevEnd)
	if err == nil {
		prevList := snapshotsInRange(prevCache, prevStart, prevEnd)
		if len(prevList) > 0 {
			prevTotals := aggregateSimple(prevList)
			response.Comparison = &SummaryComparison{
				PreviousPeriod: MetricsDateRange{Start: prevStart, End: prevEnd.Add(24 * time.Hour)},
				MRRDelta:       response.MRR - prevTotals.MRR,
				RevenueDelta:   response.TotalRevenue - prevTotals.TotalRevenue,
				NetNewDelta:    response.NetNewSubscriptions - prevTotals.NetNew,
			}
		}
	}

	return response, nil
}

type aggregateResult struct {
	MRR          int64
	TotalRevenue int64
	NetNew       int
}

func aggregateSimple(snapshots []*models.DailyMetricsSnapshot) aggregateResult {
	var res aggregateResult
	for _, snap := range snapshots {
		res.TotalRevenue += snap.SubscriptionRevenue + snap.OneTimeRevenue
		res.NetNew += snap.NewSubscriptions - (snap.CancellationsVol + snap.CancellationsInv) + snap.Reactivations
		res.MRR = snap.MRRCents
	}
	return res
}

func sumEntitlements(snaps []*models.DailyMetricsSnapshot) int {
	total := 0
	for _, snap := range snaps {
		total += snap.EntitlementsGranted
	}
	return total
}

func snapshotsInRange(cache map[time.Time]*models.DailyMetricsSnapshot, start, end time.Time) []*models.DailyMetricsSnapshot {
	result := make([]*models.DailyMetricsSnapshot, 0, len(cache))
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if snap, ok := cache[day]; ok {
			result = append(result, snap)
		}
	}
	return result
}

func (s *AdminMetricsService) GetRevenueSeries(ctx context.Context, rng MetricsDateRange, granularity string) (*RevenueSeriesResponse, error) {
	if granularity == "" {
		granularity = "day"
	}
	startDay := truncateToDay(rng.Start)
	endDay := truncateToDay(rng.End.Add(-time.Nanosecond))
	cache, err := s.loadSnapshots(ctx, startDay, endDay)
	if err != nil {
		return nil, err
	}

	response := &RevenueSeriesResponse{
		Currency:    "usd",
		Granularity: granularity,
	}

	for bucketStart := startDay; !bucketStart.After(endDay); {
		next := advance(bucketStart, granularity)
		if next.After(endDay.Add(24 * time.Hour)) {
			next = endDay.Add(24 * time.Hour)
		}
		bucket := RevenueSeriesBucket{
			PeriodStart: bucketStart,
			PeriodEnd:   next,
		}
		for day := bucketStart; day.Before(next); day = day.AddDate(0, 0, 1) {
			if day.After(endDay) {
				break
			}
			snap, ok := cache[day]
			if !ok {
				continue
			}
			dayRevenue := snap.SubscriptionRevenue + snap.OneTimeRevenue
			bucket.TotalRevenue += dayRevenue
			bucket.SubscriptionRevenue += snap.SubscriptionRevenue
			bucket.OneTimeRevenue += snap.OneTimeRevenue
			bucket.Refunds += snap.RefundsCents
			bucket.Chargebacks += snap.ChargebacksCents
			paymentCount := successfulPaymentsForDay(snap)
			bucket.Payments.Count += paymentCount
		}
		if bucket.Payments.Count > 0 {
			bucket.Payments.AverageAmount = bucket.TotalRevenue / int64(bucket.Payments.Count)
		}
		response.Buckets = append(response.Buckets, bucket)
		bucketStart = next
	}

	for _, bucket := range response.Buckets {
		response.Totals.TotalRevenue += bucket.TotalRevenue
		response.Totals.SubscriptionRevenue += bucket.SubscriptionRevenue
		response.Totals.OneTimeRevenue += bucket.OneTimeRevenue
		response.Totals.Refunds += bucket.Refunds
		response.Totals.Chargebacks += bucket.Chargebacks
	}

	return response, nil
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

func successfulPaymentsForDay(snap *models.DailyMetricsSnapshot) int {
	total := 0
	for _, proc := range snap.ProcessorBreakdowns {
		total += proc.Payments.Successful
	}
	return total
}

func (s *AdminMetricsService) GetSubscriptionSeries(ctx context.Context, rng MetricsDateRange, granularity string) (*SubscriptionSeriesResponse, error) {
	if granularity == "" {
		granularity = "day"
	}
	startDay := truncateToDay(rng.Start)
	endDay := truncateToDay(rng.End.Add(-time.Nanosecond))
	cache, err := s.loadSnapshots(ctx, startDay, endDay)
	if err != nil {
		return nil, err
	}

	response := &SubscriptionSeriesResponse{Granularity: granularity}
	for bucketStart := startDay; !bucketStart.After(endDay); {
		next := advance(bucketStart, granularity)
		if next.After(endDay.Add(24 * time.Hour)) {
			next = endDay.Add(24 * time.Hour)
		}
		bucket := SubscriptionSeriesBucket{
			PeriodStart: bucketStart,
			PeriodEnd:   next,
		}
		for day := bucketStart; day.Before(next); day = day.AddDate(0, 0, 1) {
			if day.After(endDay) {
				break
			}
			snap, ok := cache[day]
			if !ok {
				continue
			}
			bucket.NewSubscriptions += snap.NewSubscriptions
			bucket.ScheduledStarts += snap.ScheduledStarts
			bucket.Cancellations.Voluntary += snap.CancellationsVol
			bucket.Cancellations.Involuntary += snap.CancellationsInv
			bucket.Reactivations += snap.Reactivations
			bucket.ActiveCountEnd = snap.ActiveCountEnd
		}
		bucket.NetChange = bucket.NewSubscriptions - (bucket.Cancellations.Voluntary + bucket.Cancellations.Involuntary) + bucket.Reactivations
		response.Buckets = append(response.Buckets, bucket)
		bucketStart = next
	}

	for _, bucket := range response.Buckets {
		response.Totals.NewSubscriptions += bucket.NewSubscriptions
		response.Totals.Cancellations += bucket.Cancellations.Voluntary + bucket.Cancellations.Involuntary
		response.Totals.Reactivations += bucket.Reactivations
		response.Totals.NetChange += bucket.NetChange
	}

	return response, nil
}

func (s *AdminMetricsService) GetProcessorMetrics(ctx context.Context, rng MetricsDateRange) (*ProcessorMetricsResponse, error) {
	startDay := truncateToDay(rng.Start)
	endDay := truncateToDay(rng.End.Add(-time.Nanosecond))
	cache, err := s.loadSnapshots(ctx, startDay, endDay)
	if err != nil {
		return nil, err
	}

	accumulated := make(map[string]models.DailyProcessorMetrics)
	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		snap, ok := cache[day]
		if !ok {
			continue
		}
		for processor, metrics := range snap.ProcessorBreakdowns {
			entry := accumulated[processor]
			entry.ActiveSubscriptions = metrics.ActiveSubscriptions
			entry.NewSubscriptions += metrics.NewSubscriptions
			entry.Cancellations += metrics.Cancellations
			entry.Revenue.Total += metrics.Revenue.Total
			entry.Revenue.Subscription += metrics.Revenue.Subscription
			entry.Revenue.OneTime += metrics.Revenue.OneTime
			entry.Revenue.Refunds += metrics.Revenue.Refunds
			entry.Revenue.Chargebacks += metrics.Revenue.Chargebacks
			entry.Payments.Successful += metrics.Payments.Successful
			entry.Payments.Failed += metrics.Payments.Failed
			accumulated[processor] = entry
		}
	}

	response := &ProcessorMetricsResponse{
		PeriodStart: startDay,
		PeriodEnd:   endDay.Add(24 * time.Hour),
	}

	for processor, metrics := range accumulated {
		response.Processors = append(response.Processors, ProcessorMetrics{
			Processor: processor,
			Metrics:   metrics,
		})
	}

	sort.Slice(response.Processors, func(i, j int) bool {
		return response.Processors[i].Processor < response.Processors[j].Processor
	})

	return response, nil
}

func (s *AdminMetricsService) GetChurn(ctx context.Context, rng MetricsDateRange) (*ChurnResponse, error) {
	startMonth := firstOfMonth(rng.Start)
	endMonth := firstOfMonth(rng.End)
	cache, err := s.loadSnapshots(ctx, startMonth, endMonth.AddDate(0, 1, -1))
	if err != nil {
		return nil, err
	}

	response := &ChurnResponse{
		PeriodStart: startMonth,
		PeriodEnd:   endMonth.AddDate(0, 1, 0),
	}

	prevActive := 0
	reasonCounts := map[string]int{
		"user":       0,
		"merchant":   0,
		"expired":    0,
		"chargeback": 0,
	}

	for month := startMonth; !month.After(endMonth); month = month.AddDate(0, 1, 0) {
		next := month.AddDate(0, 1, 0)
		var cancelsVol, cancelsInv, activeEnd int
		for day := month; day.Before(next); day = day.AddDate(0, 0, 1) {
			snap, ok := cache[truncateToDay(day)]
			if !ok {
				continue
			}
			cancelsVol += snap.CancellationsVol
			cancelsInv += snap.CancellationsInv
			activeEnd = snap.ActiveCountEnd
		}

		activeStart := prevActive
		if activeStart == 0 {
			firstSnap, ok := cache[truncateToDay(month)]
			if ok {
				activeStart = firstSnap.ActiveCountEnd + cancelsVol + cancelsInv
			}
		}

		totalCancels := cancelsVol + cancelsInv
		var churnRate, volRate, invRate float64
		if activeStart > 0 {
			churnRate = float64(totalCancels) / float64(activeStart)
			volRate = float64(cancelsVol) / float64(activeStart)
			invRate = float64(cancelsInv) / float64(activeStart)
		}

		response.MonthlyChurn = append(response.MonthlyChurn, MonthlyChurnPoint{
			Month:           month.Format("2006-01"),
			ChurnRate:       churnRate,
			VoluntaryRate:   volRate,
			InvoluntaryRate: invRate,
			ActiveStart:     activeStart,
			ActiveEnd:       activeEnd,
		})

		reasonCounts["user"] += cancelsVol
		reasonCounts["expired"] += cancelsInv

		prevActive = activeEnd
	}

	for reason, count := range reasonCounts {
		if count == 0 {
			continue
		}
		response.CancellationReasons = append(response.CancellationReasons, ReasonCount{
			Reason: reason,
			Count:  count,
		})
	}

	response.CohortRetention = buildCohortRetention(cache, startMonth, endMonth)

	return response, nil
}

func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func buildCohortRetention(cache map[time.Time]*models.DailyMetricsSnapshot, startMonth, endMonth time.Time) []CohortRetentionEntry {
	var cohorts []CohortRetentionEntry
	for month := startMonth; !month.After(endMonth); month = month.AddDate(0, 1, 0) {
		next := month.AddDate(0, 1, 0)
		initial := 0
		activeEnd := 0
		for day := month; day.Before(next); day = day.AddDate(0, 0, 1) {
			if snap, ok := cache[truncateToDay(day)]; ok {
				initial += snap.NewSubscriptions
				activeEnd = snap.ActiveCountEnd
			}
		}
		if initial == 0 {
			continue
		}
		cohort := CohortRetentionEntry{
			Cohort:         month.Format("2006-01"),
			InitialSignups: initial,
		}
		cohort.Retention = append(cohort.Retention, CohortRetentionPoint{
			Month:  1,
			Active: activeEnd,
			Rate:   rate(activeEnd, initial),
		})
		cohorts = append(cohorts, cohort)
	}
	return cohorts
}

func rate(val, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(val) / float64(total)
}

func (s *AdminMetricsService) populateSnapshotSubscriptions(
	ctx context.Context,
	db bun.IDB,
	snapshot *models.DailyMetricsSnapshot,
	start, end time.Time,
) error {
	type countRow struct {
		Value int64
	}

	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.created_at >= ? AND sub.created_at < ?", start, end).
		Scan(ctx, &snapshot.NewSubscriptions); err != nil && err != sql.ErrNoRows {
		return err
	}

	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusPending).
		Where("sub.started_at >= ? AND sub.started_at < ?", start, end).
		Scan(ctx, &snapshot.ScheduledStarts); err != nil && err != sql.ErrNoRows {
		return err
	}

	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.cancelled_at >= ? AND sub.cancelled_at < ?", start, end).
		Where("sub.cancel_type IN (?)", bun.In([]models.CancelType{
			models.CancelTypeUser,
			models.CancelTypeMerchant,
		})).
		Scan(ctx, &snapshot.CancellationsVol); err != nil && err != sql.ErrNoRows {
		return err
	}

	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.cancelled_at >= ? AND sub.cancelled_at < ?", start, end).
		Where("sub.cancel_type IN (?)", bun.In([]models.CancelType{
			models.CancelTypeExpired,
			models.CancelTypeChargeback,
		})).
		Scan(ctx, &snapshot.CancellationsInv); err != nil && err != sql.ErrNoRows {
		return err
	}

	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusActive).
		Where("sub.updated_at >= ? AND sub.updated_at < ?", start, end).
		Where("sub.cancelled_at IS NOT NULL").
		Scan(ctx, &snapshot.Reactivations); err != nil && err != sql.ErrNoRows {
		return err
	}

	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status IN (?)", bun.In([]models.SubscriptionStatus{models.StatusActive, models.StatusPastDue})).
		Scan(ctx, &snapshot.ActiveCountEnd); err != nil && err != sql.ErrNoRows {
		return err
	}

	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusPastDue).
		Scan(ctx, &snapshot.PastDueCountEnd); err != nil && err != sql.ErrNoRows {
		return err
	}

	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusPending).
		Scan(ctx, &snapshot.PendingCountEnd); err != nil && err != sql.ErrNoRows {
		return err
	}

	mrr, err := s.currentMRR(ctx, db)
	if err != nil {
		return err
	}
	snapshot.MRRCents = mrr

	return nil
}

func (s *AdminMetricsService) currentMRR(ctx context.Context, db bun.IDB) (int64, error) {
	var mrr sql.NullInt64
	err := db.NewSelect().
		TableExpr("billing.subscriptions AS sub").
		Join("JOIN billing.prices AS price ON price.id = sub.price_id").
		ColumnExpr(`
            COALESCE(SUM(
                CASE
                    WHEN price.billing_cycle_days IS NULL OR price.billing_cycle_days = 0 THEN 0
                    ELSE (price.amount * 30) / price.billing_cycle_days
                END
            ), 0)`).
		Where("sub.status IN (?)", bun.In([]models.SubscriptionStatus{models.StatusActive, models.StatusPastDue})).
		Scan(ctx, &mrr)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if !mrr.Valid {
		return 0, nil
	}
	return mrr.Int64, nil
}

func (s *AdminMetricsService) populateSnapshotProcessors(
	ctx context.Context,
	db bun.IDB,
	snapshot *models.DailyMetricsSnapshot,
	start, end time.Time,
) error {
	type revenueRow struct {
		Processor           string
		TotalRevenue        int64
		SubscriptionRevenue int64
		OneTimeRevenue      int64
		Refunds             int64
		Chargebacks         int64
		SuccessCount        int64
	}

	var revenueRows []revenueRow
	if err := db.NewSelect().
		Model((*models.Payment)(nil)).
		ColumnExpr("purch.processor AS processor").
		ColumnExpr("COALESCE(SUM(CASE WHEN purch.amount > 0 THEN purch.amount ELSE 0 END),0) AS total_revenue").
		ColumnExpr("COALESCE(SUM(CASE WHEN purch.amount > 0 AND purch.subscription_id IS NOT NULL THEN purch.amount ELSE 0 END),0) AS subscription_revenue").
		ColumnExpr("COALESCE(SUM(CASE WHEN purch.amount > 0 AND purch.subscription_id IS NULL THEN purch.amount ELSE 0 END),0) AS one_time_revenue").
		ColumnExpr("COALESCE(SUM(CASE WHEN purch.amount < 0 AND purch.refunded_payment_id IS NOT NULL THEN ABS(purch.amount) ELSE 0 END),0) AS refunds").
		ColumnExpr("COALESCE(SUM(CASE WHEN purch.amount < 0 AND purch.refunded_payment_id IS NULL THEN ABS(purch.amount) ELSE 0 END),0) AS chargebacks").
		ColumnExpr("COALESCE(SUM(CASE WHEN purch.amount > 0 THEN 1 ELSE 0 END),0) AS success_count").
		Where("purchased_at >= ? AND purchased_at < ?", start, end).
		GroupExpr("purch.processor").
		Scan(ctx, &revenueRows); err != nil && err != sql.ErrNoRows {
		return err
	}

	type countRow struct {
		Processor string
		Count     int64
	}

	perProcessorActive := make(map[string]*models.DailyProcessorMetrics)

	attach := func(proc string) *models.DailyProcessorMetrics {
		if _, ok := perProcessorActive[proc]; !ok {
			perProcessorActive[proc] = &models.DailyProcessorMetrics{
				Revenue:  models.DailyProcessorRevenue{},
				Payments: models.DailyProcessorPayment{},
			}
		}
		return perProcessorActive[proc]
	}

	for _, row := range revenueRows {
		entry := attach(row.Processor)
		entry.Revenue.Total = row.TotalRevenue
		entry.Revenue.Subscription = row.SubscriptionRevenue
		entry.Revenue.OneTime = row.OneTimeRevenue
		entry.Revenue.Refunds = row.Refunds
		entry.Revenue.Chargebacks = row.Chargebacks
		entry.Payments.Successful = int(row.SuccessCount)
	}

	var activeRows []countRow
	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("sub.processor AS processor").
		ColumnExpr("COUNT(*) AS count").
		Where("sub.status IN (?)", bun.In([]models.SubscriptionStatus{models.StatusActive, models.StatusPastDue})).
		GroupExpr("sub.processor").
		Scan(ctx, &activeRows); err != nil && err != sql.ErrNoRows {
		return err
	}
	for _, row := range activeRows {
		entry := attach(row.Processor)
		entry.ActiveSubscriptions = int(row.Count)
	}

	var newRows []countRow
	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("sub.processor AS processor").
		ColumnExpr("COUNT(*) AS count").
		Where("sub.created_at >= ? AND sub.created_at < ?", start, end).
		GroupExpr("sub.processor").
		Scan(ctx, &newRows); err != nil && err != sql.ErrNoRows {
		return err
	}
	for _, row := range newRows {
		entry := attach(row.Processor)
		entry.NewSubscriptions = int(row.Count)
	}

	var cancelRows []countRow
	if err := db.NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("sub.processor AS processor").
		ColumnExpr("COUNT(*) AS count").
		Where("sub.cancelled_at >= ? AND sub.cancelled_at < ?", start, end).
		GroupExpr("sub.processor").
		Scan(ctx, &cancelRows); err != nil && err != sql.ErrNoRows {
		return err
	}
	for _, row := range cancelRows {
		entry := attach(row.Processor)
		entry.Cancellations = int(row.Count)
	}

	for proc, data := range perProcessorActive {
		snapshot.ProcessorBreakdowns[proc] = *data
	}

	return nil
}
