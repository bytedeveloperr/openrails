# Billing Service Integration Guide

This guide explains how to integrate the self-hosted billing service with your application.

## Architecture Overview

The billing service uses a **dual-database architecture**:
- **Billing Database**: Stores all billing data (subscriptions, payments, etc.)
- **Your Application Database**: The billing service calls functions here to manage user roles

```
Your App DB ← Billing Service → Billing DB
    ↑              ↑
    └── SQL Functions
```

## Quick Start

### 1. Set Up Billing Database

```bash
# Create billing database
createdb billing

# Run billing migrations
psql -d billing < migrations/00001_billing.up.sql
```

### 2. Install Role Functions in Your App Database

Run this migration on YOUR application's database (not the billing DB):

```bash
psql -d your_app_db < migrations/00002_billing_user_roles_interface.up.sql
```

This creates three functions:
- `billing_open_or_ensure_user_role()` - Grant roles on subscription
- `billing_extend_user_role()` - Extend role duration
- `billing_close_user_role()` - Revoke roles on cancellation

### 3. Create Billing User in Your App DB

```sql
-- In your app database
CREATE USER billing_service_user WITH LOGIN PASSWORD 'secure_password';
GRANT billing_writer TO billing_service_user;
```

### 4. Configure Billing Service

Set environment variables:

```bash
# Billing database (where billing data lives)
DATABASE_URL=postgres://user:pass@localhost/billing

# Your app database (where user/role data lives)
EXTERNAL_DATABASE_URL=postgres://billing_service_user:pass@localhost/your_app_db

# Other config
JWT_SECRET=your-secret
REDIS_URL=redis://localhost:6379
```

Or use `config.yaml`:

```yaml
db:
  url: postgres://user:pass@localhost/billing
  
external_db:
  url: postgres://billing_service_user:pass@localhost/your_app_db

jwt:
  secret: your-secret
  issuer: your-app

redis:
  host: localhost:6379
```

### 5. Start the Service

```bash
# Using Docker
docker-compose up

# Or directly
./billing server
```

## Integration Patterns

### Pattern 1: Direct Database Integration (Recommended)

The billing service calls SQL functions directly in your database:

```
Subscription Created → billing_open_or_ensure_user_role('user-id', 'premium')
Renewal Success → billing_extend_user_role('user-id', 'premium', '2025-02-01')  
Cancellation → billing_close_user_role('user-id', 'premium')
```

**Benefits:**
- ✅ Transactional consistency
- ✅ No webhook delays
- ✅ Works offline
- ✅ Simple debugging

### Pattern 2: API Integration (Alternative)

If you can't use direct DB access, use the API:

```go
// Check user subscription
GET /api/v1/users/{user_id}/subscription

// Create subscription
POST /api/v1/subscriptions
{
  "user_id": "uuid",
  "price_id": "uuid"
}
```

## Required Database Schema

Your application database must have these tables:

```sql
-- Minimum required schema
CREATE TABLE users (
  id UUID PRIMARY KEY
);

CREATE TABLE roles (
  id UUID PRIMARY KEY,
  slug TEXT UNIQUE NOT NULL
);

CREATE TABLE user_roles (
  id UUID PRIMARY KEY,
  user_id UUID REFERENCES users(id),
  role_id UUID REFERENCES roles(id),
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
```

## Payment Processors

### CCBill Integration

```yaml
ccbill:
  client_acc_num: "900000"
  client_sub_acc: "0000"  
  form_id: "your-flexform-id"
  salt: "your-salt"
```

### Mobius Integration

```yaml
mobius:
  security_key: "your-key"
  tokenization_key: "your-tokenization-key"
  webhook_secret: "your-webhook-secret"
```

## Testing

### 1. Test Role Functions

```sql
-- Test in your app DB
SELECT * FROM billing_open_or_ensure_user_role(
  'test-user-id'::uuid,
  'premium',
  NULL, NULL, 'subscription', NULL, NULL
);
```

### 2. Test Billing Service Connection

```bash
# Check health
curl http://localhost:2052/health

# Create test subscription  
curl -X POST http://localhost:2052/api/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{"user_id": "test-user-id", "price_id": "price-id"}'
```

## Troubleshooting

### "Unknown role slug"
- Ensure the role exists in your `roles` table
- Example: `INSERT INTO roles (slug) VALUES ('premium');`

### "permission denied for function"
- Grant execute permissions: `GRANT EXECUTE ON FUNCTION billing_open_or_ensure_user_role TO billing_writer;`

### Connection refused
- Check `EXTERNAL_DATABASE_URL` is correct
- Verify billing user can connect: `psql -U billing_service_user -d your_app_db`

### Roles not updating
- Check idempotency keys aren't being reused
- Verify user exists in your users table
- Check billing service logs: `docker-compose logs billing`

## Production Considerations

1. **Security**
   - Use strong passwords for database users
   - Restrict `billing_writer` role to minimum permissions
   - Use SSL for database connections

2. **High Availability**
   - Run multiple billing service instances
   - Use connection pooling
   - Set up database replication

3. **Monitoring**
   - Monitor failed role operations
   - Track payment success rates
   - Alert on service downtime

## Example Integration (Node.js)

```javascript
// Check if user has active subscription
async function hasActiveSubscription(userId) {
  const result = await db.query(`
    SELECT 1 FROM user_roles ur
    JOIN roles r ON ur.role_id = r.id
    WHERE ur.user_id = $1 AND r.slug = 'premium'
  `, [userId]);
  
  return result.rows.length > 0;
}
```

## Example Integration (Go)

```go
func HasActiveSubscription(ctx context.Context, userID uuid.UUID) bool {
    var exists bool
    err := db.QueryRowContext(ctx, `
        SELECT EXISTS(
            SELECT 1 FROM user_roles ur
            JOIN roles r ON ur.role_id = r.id
            WHERE ur.user_id = $1 AND r.slug = 'premium'
        )
    `, userID).Scan(&exists)
    
    return exists && err == nil
}
```

## Support

- GitHub Issues: https://github.com/your-org/billing
- Documentation: https://docs.your-site.com/billing
- Discord: https://discord.gg/your-server