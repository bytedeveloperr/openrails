-- Add contact info columns to subscriptions for email delivery
ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS user_email TEXT,
    ADD COLUMN IF NOT EXISTS user_name TEXT;
