## NMI (White-Label) Rebilling Webhook Reference

### 1. Overview

MobiusPay—our current white-label gateway—is built on NMI’s infrastructure, and the same webhook model is shared across NMI resellers. In this system, **transaction events** communicate actual payment outcomes (approved/failed), while **recurring subscription events** only describe subscription lifecycle changes (created/updated/deleted).

When handling rebilling logic, you should listen to **transaction events**, not the recurring lifecycle ones.

---

### 2. Relevant Event Types

| Event Type                      | Description                                               | Rebill Detection                                                      |
| ------------------------------- | --------------------------------------------------------- | --------------------------------------------------------------------- |
| `transaction.sale.success`      | Triggered when a sale (charge) succeeds.                  | If `action.source == "recurring"`, this indicates a rebill succeeded. |
| `transaction.sale.failure`      | Triggered when a sale fails (e.g., card declined).        | If `action.source == "recurring"`, this indicates a rebill failed.    |
| `recurring.subscription.add`    | Subscription was created.                                 | Lifecycle only — no payment result.                                   |
| `recurring.subscription.update` | Subscription was modified (e.g., amount, next bill date). | Lifecycle only.                                                       |
| `recurring.subscription.delete` | Subscription was canceled or removed.                     | Lifecycle only.                                                       |

---

### 3. Example Payload Structure (Simplified)

```json
{
  "event_type": "transaction.sale.success",
  "event_body": {
    "transaction_id": "123456789",
    "amount": "19.99",
    "action": {
      "source": "recurring",
      "response_code": "100",
      "response_text": "Approved"
    },
    "order_id": "sub_abc123",
    "customerid": "cust_001"
  }
}
```

**Key fields:**

* `event_type`: identifies the event category.
* `action.source`: identifies how the transaction originated. Values include `api`, `virtual_terminal`, `recurring`, etc. When the value is `recurring`, it means this charge was part of an automated rebill.
* `order_id` / `ponumber` / `customerid`: merchant-defined fields used to link the transaction back to your internal subscription record.

> **Note:** The payload does **not** include a `subscription_id` field. Use merchant-supplied fields like `order_id` or `customerid` to correlate the rebill with your system’s subscription.

---

### 4. Recommended Handling Logic

**Success Path:**

```ts
if (event.event_type === "transaction.sale.success" && event.event_body.action.source === "recurring") {
  RenewMembership({
    processor: "nmi",
    order_id: event.event_body.order_id,
    amount: event.event_body.amount,
    transaction_id: event.event_body.transaction_id
  });
  // Optionally create a Payment record for bookkeeping
}
```

**Failure Path:**

```ts
if (event.event_type === "transaction.sale.failure" && event.event_body.action.source === "recurring") {
  FailMembership({
    processor: "nmi",
    order_id: event.event_body.order_id,
    reason: event.event_body.action.response_text || event.event_body.action.response_code
  });
}
```

---

### 5. Response Codes & Common Meanings

| Response Code | Meaning                                | Action               |
| ------------- | -------------------------------------- | -------------------- |
| `100`         | Approved                               | Success              |
| `200`         | Declined                               | Retry or Fail        |
| `261`         | Declined – Stop all recurring payments | Cancel subscription  |
| `262`         | Declined – Stop this recurring program | Pause or remove plan |

---

### 6. Implementation Notes

* Treat all `transaction.sale.*` events as the canonical source of truth for payment outcomes.
* Use `action.source == "recurring"` to distinguish rebills from manual payments.
* The `recurring.subscription.*` family of events is only useful for tracking lifecycle or metadata changes.
* Always carry your own unique subscription identifiers in fields like `order_id` or `ponumber` during subscription creation so they return in webhook payloads.
* NMI/NMI webhooks deliver the same JSON structure across both hosted and API-initiated transactions.
* Doujins sends the internal subscription UUID as both `order_id` and `ponumber` when creating NMI subscriptions so that `transaction.sale.*` payloads can be correlated even if the gateway omits `subscription_id`.
* The billing service now records a payment entry and issues a renewal only when `transaction.sale.*` events originate from `action.source` values of `recurring` or `retry`.

---

### 7. Summary

To detect NMI (NMI) rebills:

1. Subscribe to `transaction.sale.success` and `transaction.sale.failure` webhooks.
2. Filter events where `event_body.action.source == "recurring"`.
3. Use `order_id` or `ponumber` to map the transaction to your local subscription.
4. Treat recurring subscription events (`recurring.subscription.*`) only as lifecycle signals, not payment results.
