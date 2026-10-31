CREATE TABLE applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    api_key_hash TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE recipients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    external_user_id TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (application_id, external_user_id),
    UNIQUE (application_id, id)
);

CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    recipient_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'processed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ,
    UNIQUE (application_id, idempotency_key),
    FOREIGN KEY (application_id, recipient_id) REFERENCES recipients(application_id, id)
);
CREATE INDEX events_type_status_idx ON events (event_type, status);

CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    channel TEXT NOT NULL CHECK (channel IN ('email', 'sms', 'inapp')),
    subject TEXT,
    body TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (application_id, event_type, channel)
);

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    recipient_id UUID NOT NULL REFERENCES recipients(id),
    channel TEXT NOT NULL CHECK (channel IN ('email', 'sms', 'inapp')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'delivered', 'failed')),
    rendered_subject TEXT,
    rendered_body TEXT NOT NULL,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, channel)
);
CREATE INDEX notifications_recipient_created_idx ON notifications (recipient_id, created_at DESC);

CREATE TABLE delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed')),
    provider TEXT,
    provider_message_id TEXT,
    error_message TEXT,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (notification_id, attempt_number)
);

CREATE TABLE outbox_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    routing_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT,
    claimed_until TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished_idx ON outbox_messages (created_at) WHERE published_at IS NULL;
