-- Rebuild daily metrics with per-currency, carry-forward, and processor alignment

-- Subscription events: ensure extended fields exist
ALTER TABLE subscription_events {{ON_CLUSTER}}
    ADD COLUMN IF NOT EXISTS status LowCardinality(String) DEFAULT '',
    ADD COLUMN IF NOT EXISTS cancel_type LowCardinality(String) DEFAULT '',
    ADD COLUMN IF NOT EXISTS price_amount Decimal(12, 2) DEFAULT 0,
    ADD COLUMN IF NOT EXISTS price_currency LowCardinality(String) DEFAULT 'usd',
    ADD COLUMN IF NOT EXISTS billing_cycle_days UInt32 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS product_id Nullable(UUID),
    ADD COLUMN IF NOT EXISTS price_id Nullable(UUID);

-- Create daily_metrics if missing
CREATE TABLE IF NOT EXISTS daily_metrics {{ON_CLUSTER}} (
    snapshot_date Date,
    currency LowCardinality(String) DEFAULT 'usd',
    subscription_revenue_cents Int64,
    one_time_revenue_cents Int64,
    refunds_cents Int64,
    chargebacks_cents Int64,
    total_revenue_cents Int64,
    total_revenue_net_cents Int64,
    payments_successful Int64,
    payments_failed Int64,
    avg_payment_amount_cents Int64,
    new_subscriptions Int64,
    scheduled_starts Nullable(Int64),
    cancellations_user Int64,
    cancellations_merchant Int64,
    cancellations_expired Int64,
    cancellations_chargeback Int64,
    reactivations Int64,
    active_count_end Int64,
    past_due_count_end Int64,
    pending_count_end Int64,
    mrr_cents Int64,
    entitlements_granted Nullable(Int64),
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
ORDER BY (snapshot_date, currency)
PARTITION BY toYYYYMM(snapshot_date)
SETTINGS index_granularity = 8192;

-- Drop legacy MVs
DROP TABLE IF EXISTS mv_daily_metrics_subscriptions {{ON_CLUSTER}};
DROP TABLE IF EXISTS mv_daily_metrics_payments {{ON_CLUSTER}};
DROP TABLE IF EXISTS mv_daily_metrics {{ON_CLUSTER}};

-- Consolidated MV with date spine and per-currency carry-forward
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_daily_metrics {{ON_CLUSTER}}
TO daily_metrics
AS
WITH
event_bounds AS (
    SELECT
        coalesce(min(ts), today()) AS min_date,
        greatest(coalesce(max(ts), today()), today()) AS max_date
    FROM (
        SELECT toDate(timestamp) AS ts FROM subscription_events
        UNION ALL
        SELECT toDate(timestamp) AS ts FROM payment_events
    )
),
currencies AS (
    SELECT DISTINCT currency FROM (
        SELECT price_currency AS currency FROM subscription_events
        UNION ALL
        SELECT currency FROM payment_events
    )
),
spine AS (
    SELECT
        dateAdd(day, n, bounds.min_date) AS snapshot_date,
        c.currency AS currency
    FROM event_bounds AS bounds
    CROSS JOIN currencies AS c
    ARRAY JOIN range(toUInt32(dateDiff('day', bounds.min_date, bounds.max_date) + 1)) AS n
),
per_sub AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        if(price_currency = '', 'usd', price_currency) AS currency,
        subscription_id,
        argMax(status, timestamp) AS status,
        argMax(price_amount, timestamp) AS price_amount,
        argMax(billing_cycle_days, timestamp) AS billing_cycle_days,
        argMax(processor, timestamp) AS processor
    FROM subscription_events
    GROUP BY snapshot_date, currency, subscription_id
),
sub_status AS (
    SELECT
        snapshot_date,
        currency,
        countIf(status = 'active') AS active_count_end,
        countIf(status = 'past_due') AS past_due_count_end,
        countIf(status = 'pending') AS pending_count_end,
        toInt64(sumIf(price_amount * 100 * 30 / NULLIF(billing_cycle_days,0), status IN ('active','past_due'))) AS mrr_cents
    FROM per_sub
    GROUP BY snapshot_date, currency
),
sub_events AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        if(price_currency = '', 'usd', price_currency) AS currency,
        countIf(event_type = 'subscription_created') AS new_subscriptions,
        countIf(event_type = 'subscription_created' AND status = 'pending') AS scheduled_starts,
        countIf(event_type = 'subscription_reactivated') AS reactivations,
        countIf(event_type = 'subscription_cancelled' AND cancel_type = 'user') AS cancellations_user,
        countIf(event_type = 'subscription_cancelled' AND cancel_type = 'merchant') AS cancellations_merchant,
        countIf(event_type = 'subscription_expired' OR (event_type = 'subscription_cancelled' AND cancel_type = 'expired')) AS cancellations_expired,
        countIf(event_type = 'subscription_cancelled' AND cancel_type = 'chargeback') AS cancellations_chargeback,
        countIf(event_type IN ('subscription_created','subscription_reactivated') AND status IN ('active','past_due')) AS entitlements_granted
    FROM subscription_events
    GROUP BY snapshot_date, currency
),
sub_proc AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        if(price_currency = '', 'usd', price_currency) AS currency,
        processor,
        toInt64(countIf(event_type = 'subscription_created')) AS new_subscriptions,
        toInt64(countIf(event_type IN ('subscription_cancelled','subscription_expired'))) AS cancellations,
        toInt64(countIf(status = 'active')) AS active_subscriptions
    FROM subscription_events
    GROUP BY snapshot_date, currency, processor
),
pay_events AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        if(currency = '', 'usd', currency) AS currency,
        toInt64(sumIf(amount * 100, event_type = 'charge_success' AND subscription_id IS NOT NULL)) AS subscription_revenue_cents,
        toInt64(sumIf(amount * 100, event_type = 'charge_success' AND subscription_id IS NULL)) AS one_time_revenue_cents,
        toInt64(sumIf(abs(amount) * 100, event_type = 'refund')) AS refunds_cents,
        toInt64(sumIf(abs(amount) * 100, event_type = 'chargeback')) AS chargebacks_cents,
        toInt64(sumIf(amount * 100, event_type = 'charge_success')) AS total_revenue_cents,
        toInt64(countIf(event_type = 'charge_success')) AS payments_successful,
        toInt64(countIf(event_type = 'charge_failure')) AS payments_failed
    FROM payment_events
    GROUP BY snapshot_date, currency
),
pay_proc AS (
    SELECT
        toDate(timestamp) AS snapshot_date,
        if(currency = '', 'usd', currency) AS currency,
        processor,
        toInt64(sumIf(amount * 100, event_type = 'charge_success')) AS revenue_total_cents,
        toInt64(sumIf(amount * 100, event_type = 'charge_success' AND subscription_id IS NOT NULL)) AS revenue_subscription_cents,
        toInt64(sumIf(amount * 100, event_type = 'charge_success' AND subscription_id IS NULL)) AS revenue_one_time_cents,
        toInt64(sumIf(abs(amount) * 100, event_type = 'refund')) AS revenue_refunds_cents,
        toInt64(sumIf(abs(amount) * 100, event_type = 'chargeback')) AS revenue_chargebacks_cents,
        toInt64(countIf(event_type = 'charge_success')) AS payments_successful,
        toInt64(countIf(event_type = 'charge_failure')) AS payments_failed
    FROM payment_events
    GROUP BY snapshot_date, currency, processor
),
proc_join AS (
    SELECT
        coalesce(sp.snapshot_date, pp.snapshot_date) AS snapshot_date,
        coalesce(sp.currency, pp.currency) AS currency,
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
      ON sp.snapshot_date = pp.snapshot_date
     AND sp.processor = pp.processor
     AND sp.currency = pp.currency
),
proc_arrays AS (
    -- build processor spine per currency/day, carry forward actives to avoid dropping processors on days without data
    WITH
    proc_dim AS (
        SELECT DISTINCT currency, processor FROM proc_join
    ),
    proc_spine AS (
        SELECT s.snapshot_date, d.currency, d.processor
        FROM spine s
        INNER JOIN proc_dim d ON s.currency = d.currency
    ),
    proc_filled AS (
        SELECT
            ps.snapshot_date,
            ps.currency,
            ps.processor,
            pj.active_subscriptions AS active_subscriptions_raw,
            coalesce(pj.new_subscriptions, 0) AS new_subscriptions,
            coalesce(pj.cancellations, 0) AS cancellations,
            coalesce(pj.revenue_total_cents, 0) AS revenue_total_cents,
            coalesce(pj.revenue_subscription_cents, 0) AS revenue_subscription_cents,
            coalesce(pj.revenue_one_time_cents, 0) AS revenue_one_time_cents,
            coalesce(pj.revenue_refunds_cents, 0) AS revenue_refunds_cents,
            coalesce(pj.revenue_chargebacks_cents, 0) AS revenue_chargebacks_cents,
            coalesce(pj.payments_successful, 0) AS payments_successful,
            coalesce(pj.payments_failed, 0) AS payments_failed
        FROM proc_spine ps
        LEFT JOIN proc_join pj
          ON ps.snapshot_date = pj.snapshot_date
         AND ps.currency = pj.currency
         AND ps.processor = pj.processor
    ),
proc_carried AS (
    SELECT
        snapshot_date,
        currency,
        processor,
        last_value(active_subscriptions_raw) IGNORE NULLS OVER (PARTITION BY currency, processor ORDER BY snapshot_date ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS active_subscriptions,
        new_subscriptions,
        cancellations,
        revenue_total_cents,
        revenue_subscription_cents,
        revenue_one_time_cents,
        revenue_refunds_cents,
        revenue_chargebacks_cents,
        payments_successful,
        payments_failed
    FROM proc_filled
)
SELECT
    snapshot_date,
    currency,
    arraySort(groupArray((processor,
        coalesce(active_subscriptions, 0),
        coalesce(new_subscriptions, 0),
        coalesce(cancellations, 0),
        coalesce(revenue_total_cents, 0),
        coalesce(revenue_subscription_cents, 0),
        coalesce(revenue_one_time_cents, 0),
        coalesce(revenue_refunds_cents, 0),
        coalesce(revenue_chargebacks_cents, 0),
        coalesce(payments_successful, 0),
        coalesce(payments_failed, 0)))) AS proc_sorted
FROM proc_carried
GROUP BY snapshot_date, currency
),
-- Merge daily aggregates per currency
daily AS (
    SELECT
        s.snapshot_date,
        s.currency,
        coalesce(pe.subscription_revenue_cents, 0) AS subscription_revenue_cents,
        coalesce(pe.one_time_revenue_cents, 0) AS one_time_revenue_cents,
        coalesce(pe.refunds_cents, 0) AS refunds_cents,
        coalesce(pe.chargebacks_cents, 0) AS chargebacks_cents,
    coalesce(pe.total_revenue_cents, 0) AS total_revenue_cents,
    coalesce(pe.payments_successful, 0) AS payments_successful,
    coalesce(pe.payments_failed, 0) AS payments_failed,
    coalesce(se.new_subscriptions, 0) AS new_subscriptions,
    coalesce(se.reactivations, 0) AS reactivations,
    coalesce(se.cancellations_user, 0) AS cancellations_user,
    coalesce(se.cancellations_merchant, 0) AS cancellations_merchant,
    coalesce(se.cancellations_expired, 0) AS cancellations_expired,
    coalesce(se.cancellations_chargeback, 0) AS cancellations_chargeback,
    ss.active_count_end AS active_count_end,
    ss.past_due_count_end AS past_due_count_end,
    ss.pending_count_end AS pending_count_end,
    ss.mrr_cents AS mrr_cents,
        arrayMap(x -> x.1, pa.proc_sorted) AS proc_name,
        arrayMap(x -> x.2, pa.proc_sorted) AS proc_active,
        arrayMap(x -> x.3, pa.proc_sorted) AS proc_new,
        arrayMap(x -> x.4, pa.proc_sorted) AS proc_cancel,
        arrayMap(x -> x.5, pa.proc_sorted) AS proc_rev_total,
        arrayMap(x -> x.6, pa.proc_sorted) AS proc_rev_sub,
        arrayMap(x -> x.7, pa.proc_sorted) AS proc_rev_one_time,
        arrayMap(x -> x.8, pa.proc_sorted) AS proc_refunds,
        arrayMap(x -> x.9, pa.proc_sorted) AS proc_chargebacks,
        arrayMap(x -> x.10, pa.proc_sorted) AS proc_pay_success,
        arrayMap(x -> x.11, pa.proc_sorted) AS proc_pay_failed
    FROM spine s
    LEFT JOIN sub_events se ON s.snapshot_date = se.snapshot_date AND s.currency = se.currency
    LEFT JOIN sub_status ss ON s.snapshot_date = ss.snapshot_date AND s.currency = ss.currency
    LEFT JOIN pay_events pe ON s.snapshot_date = pe.snapshot_date AND s.currency = pe.currency
    LEFT JOIN proc_arrays pa ON s.snapshot_date = pa.snapshot_date AND s.currency = pa.currency
)
SELECT
    snapshot_date,
    currency,
    subscription_revenue_cents,
    one_time_revenue_cents,
    refunds_cents,
    chargebacks_cents,
    total_revenue_cents,
    total_revenue_cents - refunds_cents - chargebacks_cents AS total_revenue_net_cents,
    payments_successful,
    payments_failed,
    if(payments_successful = 0, 0, toInt64((total_revenue_cents - refunds_cents - chargebacks_cents) / payments_successful)) AS avg_payment_amount_cents,
    new_subscriptions,
    coalesce(se.scheduled_starts, 0) AS scheduled_starts,
    cancellations_user,
    cancellations_merchant,
    cancellations_expired,
    cancellations_chargeback,
    reactivations,
    -- carry-forward status counts and MRR
    coalesce(last_value(active_count_end) IGNORE NULLS OVER w, 0) AS active_count_end,
    coalesce(last_value(past_due_count_end) IGNORE NULLS OVER w, 0) AS past_due_count_end,
    coalesce(last_value(pending_count_end) IGNORE NULLS OVER w, 0) AS pending_count_end,
    coalesce(last_value(mrr_cents) IGNORE NULLS OVER w, 0) AS mrr_cents,
    coalesce(se.entitlements_granted, 0) AS entitlements_granted,
    -- processor nested arrays aligned (actives carried in proc_carried, flows remain per-day)
    proc_name,
    proc_active,
    proc_new,
    proc_cancel,
    proc_rev_total,
    proc_rev_sub,
    proc_rev_one_time,
    proc_refunds,
    proc_chargebacks,
    proc_pay_success,
    proc_pay_failed,
    now('UTC') AS created_at,
    now('UTC') AS version
FROM daily
WINDOW w AS (PARTITION BY currency ORDER BY snapshot_date ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW);
