-- ClickHouse Schema
-- Migration 000002: Analytics tables

CREATE DATABASE IF NOT EXISTS swiftmail;

-- Main events table (used by analytics batcher)
CREATE TABLE IF NOT EXISTS swiftmail.email_events (
    user_id UUID,
    email_id UUID,
    domain_id UUID,
    event_type String,
    recipient String,
    ip_address String,
    user_agent String,
    timestamp DateTime
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (user_id, timestamp, event_type)
TTL timestamp + INTERVAL 365 DAY;

-- Aggregated email stats (for fast dashboard queries)
CREATE TABLE IF NOT EXISTS swiftmail.email_stats (
    user_id UUID,
    domain String,
    event_type String,
    tag String,
    count UInt64,
    event_date Date,
    event_hour UInt8,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(event_date)
ORDER BY (user_id, event_date, event_type)
TTL event_date + INTERVAL 365 DAY;

-- Click analytics
CREATE TABLE IF NOT EXISTS swiftmail.click_stats (
    user_id UUID,
    email_log_id UUID,
    url String,
    country String,
    device String,
    browser String,
    os String,
    clicked_at DateTime,
    event_date Date
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(event_date)
ORDER BY (user_id, event_date)
TTL event_date + INTERVAL 365 DAY;

-- Bounce analytics
CREATE TABLE IF NOT EXISTS swiftmail.bounce_stats (
    user_id UUID,
    domain String,
    bounce_type String,
    bounce_code String,
    recipient_domain String,
    event_date Date,
    count UInt64
) ENGINE = SummingMergeTree(count)
PARTITION BY toYYYYMM(event_date)
ORDER BY (user_id, event_date, recipient_domain, bounce_type)
TTL event_date + INTERVAL 365 DAY;
