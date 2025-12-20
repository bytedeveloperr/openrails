-- Add pricing/status fields to subscription_events if they are missing
ALTER TABLE subscription_events {{ON_CLUSTER}}
    ADD COLUMN IF NOT EXISTS status LowCardinality(String) DEFAULT '',
    ADD COLUMN IF NOT EXISTS cancel_type LowCardinality(String) DEFAULT '',
    ADD COLUMN IF NOT EXISTS price_amount Decimal(12, 2) DEFAULT 0,
    ADD COLUMN IF NOT EXISTS price_currency LowCardinality(String) DEFAULT 'usd',
    ADD COLUMN IF NOT EXISTS billing_cycle_days UInt32 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS product_id Nullable(UUID),
    ADD COLUMN IF NOT EXISTS price_id Nullable(UUID);

-- Create daily_metrics if it does not yet exist
CREATE TABLE IF NOT EXISTS daily_metrics {{ON_CLUSTER}} (
    snapshot_date Date,
    currency LowCardinality(String) DEFAULT 'usd',
    subscription_revenue_cents Int64,
    one_time_revenue_cents Int64,
    refunds_cents Int64,
    chargebacks_cents Int64,
    total_revenue_cents Int64,
    payments_successful Int64,
    payments_failed Int64,
    avg_payment_amount_cents Int64,
    new_subscriptions Int64,
    scheduled_starts Int64,
    cancellations_user Int64,
    cancellations_merchant Int64,
    cancellations_expired Int64,
    cancellations_chargeback Int64,
    reactivations Int64,
    active_count_end Int64,
    past_due_count_end Int64,
    pending_count_end Int64,
    mrr_cents Int64,
    entitlements_granted Int64,
    processor Nested (
        name String,
        active_subscriptions Int64,
        new_subscriptions Int64,
        cancellations Int64,
        revenue_total_cents Int64,
        revenue_subscription_cents Int64,
        revenue_one_time_cents Int64,
        revenue_refunds_cents Int64,
        revenue_chargebacks_cents Int64,
        payments_successful Int64,
        payments_failed Int64
    ),
    created_at DateTime('UTC') DEFAULT now(),
    version DateTime('UTC') DEFAULT now()
) ENGINE = ReplicatedReplacingMergeTree('/clickhouse/tables/{database}/{table}', '{replica}', version)
ORDER BY (snapshot_date)
PARTITION BY toYYYYMM(snapshot_date)
SETTINGS index_granularity = 8192;

-- Drop any legacy daily metrics materialized views to avoid double writes
DROP TABLE IF EXISTS mv_daily_metrics_subscriptions {{ON_CLUSTER}};
DROP TABLE IF EXISTS mv_daily_metrics_payments {{ON_CLUSTER}};
DROP TABLE IF EXISTS mv_daily_metrics {{ON_CLUSTER}};

-- Consolidated materialized view: one row per day with processor Nested aligned by processor name
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_daily_metrics {{ON_CLUSTER}}
TO daily_metrics
AS
WITH
per_sub AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        subscription_id,
        argMax(status, timestamp) AS status,
        argMax(price_amount, timestamp) AS price_amount,
        argMax(price_currency, timestamp) AS price_currency,
        argMax(billing_cycle_days, timestamp) AS billing_cycle_days,
        argMax(processor, timestamp) AS processor
    FROM subscription_events
    GROUP BY snapshot_date, subscription_id
),
sub_status AS (
    SELECT
        snapshot_date,
        any(price_currency) AS currency,
        countIf(status = 'active') AS active_count_end,
        countIf(status = 'past_due') AS past_due_count_end,
        countIf(status = 'pending') AS pending_count_end,
        toInt64(sumIf(price_amount * 100 * 30 / NULLIF(billing_cycle_days,0), status IN ('active','past_due'))) AS mrr_cents
    FROM per_sub
    GROUP BY snapshot_date
),
sub_events AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        countIf(event_type = 'subscription_created') AS new_subscriptions,
        countIf(event_type = 'subscription_reactivated') AS reactivations,
        countIf(event_type = 'subscription_cancelled' AND cancel_type = 'user') AS cancellations_user,
        countIf(event_type = 'subscription_cancelled' AND cancel_type = 'merchant') AS cancellations_merchant,
        countIf(event_type = 'subscription_expired' OR (event_type = 'subscription_cancelled' AND cancel_type = 'expired')) AS cancellations_expired,
        countIf(event_type = 'subscription_cancelled' AND cancel_type = 'chargeback') AS cancellations_chargeback
    FROM subscription_events
    GROUP BY snapshot_date
),
sub_proc AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        processor,
        toInt64(countIf(event_type = 'subscription_created')) AS new_subscriptions,
        toInt64(countIf(event_type = 'subscription_cancelled' OR event_type = 'subscription_expired')) AS cancellations,
        toInt64(countIf(status = 'active')) AS active_subscriptions
    FROM subscription_events
    GROUP BY snapshot_date, processor
),
pay_events AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        coalesce(any(currency), 'usd') AS currency_pay,
        toInt64(sumIf(amount * 100, event_type = 'charge_success' AND subscription_id IS NOT NULL)) AS subscription_revenue_cents,
        toInt64(sumIf(amount * 100, event_type = 'charge_success' AND subscription_id IS NULL)) AS one_time_revenue_cents,
        toInt64(sumIf(abs(amount) * 100, event_type = 'refund')) AS refunds_cents,
        toInt64(sumIf(abs(amount) * 100, event_type = 'chargeback')) AS chargebacks_cents,
        toInt64(sumIf(amount * 100, event_type = 'charge_success')) AS total_revenue_cents,
        toInt64(countIf(event_type = 'charge_success')) AS payments_successful,
        toInt64(countIf(event_type = 'charge_failure')) AS payments_failed
    FROM payment_events
    GROUP BY snapshot_date
),
pay_proc AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        processor,
        toInt64(sumIf(amount * 100, event_type = 'charge_success')) AS revenue_total_cents,
        toInt64(sumIf(amount * 100, event_type = 'charge_success' AND subscription_id IS NOT NULL)) AS revenue_subscription_cents,
        toInt64(sumIf(amount * 100, event_type = 'charge_success' AND subscription_id IS NULL)) AS revenue_one_time_cents,
        toInt64(sumIf(abs(amount) * 100, event_type = 'refund')) AS revenue_refunds_cents,
        toInt64(sumIf(abs(amount) * 100, event_type = 'chargeback')) AS revenue_chargebacks_cents,
        toInt64(countIf(event_type = 'charge_success')) AS payments_successful,
        toInt64(countIf(event_type = 'charge_failure')) AS payments_failed
    FROM payment_events
    GROUP BY snapshot_date, processor
),
proc_join AS (
    SELECT
        coalesce(sp.snapshot_date, pp.snapshot_date) AS snapshot_date,
        coalesce(sp.processor, pp.processor) AS processor,
        coalesce(sp.active_subscriptions, 0) AS active_subscriptions,
        coalesce(sp.new_subscriptions, 0) AS new_subscriptions,
        coalesce(sp.cancellations, 0) AS cancellations,
        coalesce(pp.revenue_total_cents, 0) AS revenue_total_cents,
        coalesce(pp.revenue_subscription_cents, 0) AS revenue_subscription_cents,
        coalesce(pp.revenue_one_time_cents, 0) AS revenue_one_time_cents,
        coalesce(pp.revenue_refunds_cents, 0) AS revenue_refunds_cents,
        coalesce(pp.revenue_chargebacks_cents, 0) AS revenue_chargebacks_cents,
        coalesce(pp.payments_successful, 0) AS payments_successful,
        coalesce(pp.payments_failed, 0) AS payments_failed
    FROM sub_proc sp
    FULL OUTER JOIN pay_proc pp
      ON sp.snapshot_date = pp.snapshot_date AND sp.processor = pp.processor
),
proc_arrays AS (
    SELECT
        snapshot_date,
        arraySort(groupArray((processor,
            active_subscriptions,
            new_subscriptions,
            cancellations,
            revenue_total_cents,
            revenue_subscription_cents,
            revenue_one_time_cents,
            revenue_refunds_cents,
            revenue_chargebacks_cents,
            payments_successful,
            payments_failed))) AS proc_sorted
    FROM proc_join
    GROUP BY snapshot_date
)
SELECT
    coalesce(se.snapshot_date, pe.snapshot_date, ss.snapshot_date) AS snapshot_date,
    coalesce(ss.currency, pe.currency_pay, 'usd') AS currency,
    coalesce(pe.subscription_revenue_cents, 0) AS subscription_revenue_cents,
    coalesce(pe.one_time_revenue_cents, 0) AS one_time_revenue_cents,
    coalesce(pe.refunds_cents, 0) AS refunds_cents,
    coalesce(pe.chargebacks_cents, 0) AS chargebacks_cents,
    coalesce(pe.total_revenue_cents, 0) AS total_revenue_cents,
    coalesce(pe.payments_successful, 0) AS payments_successful,
    coalesce(pe.payments_failed, 0) AS payments_failed,
    if(coalesce(pe.payments_successful, 0) = 0, 0, toInt64(pe.total_revenue_cents / pe.payments_successful)) AS avg_payment_amount_cents,
    coalesce(se.new_subscriptions, 0) AS new_subscriptions,
    0 AS scheduled_starts,
    coalesce(se.cancellations_user, 0) AS cancellations_user,
    coalesce(se.cancellations_merchant, 0) AS cancellations_merchant,
    coalesce(se.cancellations_expired, 0) AS cancellations_expired,
    coalesce(se.cancellations_chargeback, 0) AS cancellations_chargeback,
    coalesce(se.reactivations, 0) AS reactivations,
    coalesce(ss.active_count_end, 0) AS active_count_end,
    coalesce(ss.past_due_count_end, 0) AS past_due_count_end,
    coalesce(ss.pending_count_end, 0) AS pending_count_end,
    coalesce(ss.mrr_cents, 0) AS mrr_cents,
    0 AS entitlements_granted,
    -- processor nested arrays aligned and sorted by processor name
    arrayMap(x -> x.1, pa.proc_sorted) AS processor.name,
    arrayMap(x -> x.2, pa.proc_sorted) AS processor.active_subscriptions,
    arrayMap(x -> x.3, pa.proc_sorted) AS processor.new_subscriptions,
    arrayMap(x -> x.4, pa.proc_sorted) AS processor.cancellations,
    arrayMap(x -> x.5, pa.proc_sorted) AS processor.revenue_total_cents,
    arrayMap(x -> x.6, pa.proc_sorted) AS processor.revenue_subscription_cents,
    arrayMap(x -> x.7, pa.proc_sorted) AS processor.revenue_one_time_cents,
    arrayMap(x -> x.8, pa.proc_sorted) AS processor.revenue_refunds_cents,
    arrayMap(x -> x.9, pa.proc_sorted) AS processor.revenue_chargebacks_cents,
    arrayMap(x -> x.10, pa.proc_sorted) AS processor.payments_successful,
    arrayMap(x -> x.11, pa.proc_sorted) AS processor.payments_failed,
    now('UTC') AS created_at,
    now('UTC') AS version
FROM sub_events se
FULL OUTER JOIN sub_status ss ON se.snapshot_date = ss.snapshot_date
FULL OUTER JOIN pay_events pe ON coalesce(se.snapshot_date, ss.snapshot_date) = pe.snapshot_date
LEFT JOIN proc_arrays pa ON coalesce(se.snapshot_date, ss.snapshot_date, pe.snapshot_date) = pa.snapshot_date;
