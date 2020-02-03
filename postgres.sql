CREATE TABLE kv (
  k text PRIMARY KEY,
  v jsonb NOT NULL
);

CREATE TABLE accounts (
  id text PRIMARY KEY,
  lnurl_key text UNIQUE NOT NULL
);

CREATE TABLE contracts (
  id text PRIMARY KEY, -- prefix to turn into the init invoice label
  name text NOT NULL DEFAULT '',
  readme text NOT NULL DEFAULT '',
  code text NOT NULL,
  state jsonb NOT NULL DEFAULT '{}',
  created_at timestamp NOT NULL DEFAULT now(),

  CONSTRAINT state_is_object CHECK (jsonb_typeof(state) = 'object'),
  CONSTRAINT code_exists CHECK (code != '')
);

CREATE TABLE calls (
  id text PRIMARY KEY,
  time timestamp NOT NULL DEFAULT now(),
  contract_id text NOT NULL REFERENCES contracts (id) ON DELETE CASCADE,
  method text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  msatoshi int NOT NULL DEFAULT 0,
  cost int NOT NULL,
  caller text REFERENCES accounts(id),
  diff text,

  CONSTRAINT method_exists CHECK (method != ''),
  CONSTRAINT cost_positive CHECK (method = '__init__' OR cost > 0),
  CONSTRAINT caller_not_blank CHECK (caller != ''),
  CONSTRAINT msatoshi_not_negative CHECK (msatoshi >= 0)
);

CREATE INDEX IF NOT EXISTS idx_calls_by_contract ON calls (contract_id, time);

CREATE TABLE internal_transfers (
  call_id text NOT NULL REFERENCES calls (id),
  time timestamp NOT NULL DEFAULT now(),
  msatoshi int NOT NULL,
  from_contract text REFERENCES contracts(id),
  from_account text REFERENCES accounts(id),
  to_account text REFERENCES accounts(id),
  to_contract text REFERENCES contracts(id),

  CONSTRAINT one_receiver CHECK (
    (to_contract IS NOT NULL AND to_contract != '' AND to_account IS NULL) OR
    (to_contract IS NULL AND to_account IS NOT NULL AND to_account != '')
  ),
  CONSTRAINT one_sender CHECK (
    (from_contract IS NOT NULL AND from_contract != '' AND from_account IS NULL) OR
    (from_contract IS NULL AND from_account IS NOT NULL AND from_account != '')
  )
);

CREATE INDEX IF NOT EXISTS idx_internal_transfers_from_contract ON internal_transfers (from_contract);
CREATE INDEX IF NOT EXISTS idx_internal_transfers_to_contract ON internal_transfers (to_contract);
CREATE INDEX IF NOT EXISTS idx_internal_transfers_from_account ON internal_transfers (from_account);
CREATE INDEX IF NOT EXISTS idx_internal_transfers_to_account ON internal_transfers (to_account);

CREATE TABLE withdrawals (
  account_id text NOT NULL REFERENCES accounts(id),
  time timestamp NOT NULL DEFAULT now(),
  msatoshi int NOT NULL,
  fulfilled bool NOT NULL,
  bolt11 text NOT NULL
);

CREATE FUNCTION funds(contracts) RETURNS bigint AS $$
  SELECT (
    SELECT coalesce(sum(msatoshi), 0)
    FROM calls WHERE calls.contract_id = $1.id
  ) - (
    SELECT coalesce(sum(msatoshi), 0)
    FROM internal_transfers WHERE from_contract = $1.id
  ) + (
    SELECT coalesce(sum(msatoshi), 0)
    FROM internal_transfers WHERE to_contract = $1.id
  );
$$ LANGUAGE SQL;

CREATE FUNCTION balance(accounts) RETURNS bigint AS $$
  SELECT (
    SELECT coalesce(sum(msatoshi), 0)
    FROM internal_transfers WHERE to_account = $1.id
  ) - (
    SELECT coalesce(sum(msatoshi), 0)
    FROM internal_transfers WHERE from_account = $1.id
  ) - (
    SELECT coalesce(sum(msatoshi), 0)
    FROM withdrawals WHERE account_id = $1.id
  );
$$ LANGUAGE SQL;

CREATE VIEW contract_events AS
    SELECT
      contracts.id AS contract,
      'call' AS kind,
      calls.id AS call,
      time,
      msatoshi,
      jsonb_build_object('method', method, 'payload', payload, 'caller', caller) AS data
    FROM calls
    INNER JOIN contracts on calls.contract_id = contracts.id
  UNION ALL
    SELECT
      contracts.id AS contract,
      'diff' AS kind,
      calls.id AS call,
      time,
      0 AS msatoshi,
      jsonb_build_object('diff', diff) AS data
    FROM calls
    INNER JOIN contracts on calls.contract_id = contracts.id
    WHERE diff IS NOT NULL AND diff != ''
  UNION ALL
    SELECT
      contracts.id AS contract,
      'transfer_out' AS kind,
      calls.id AS call,
      calls.time AS time,
      internal_transfers.msatoshi AS msatoshi,
      jsonb_build_object('other', CASE WHEN to_account IS NOT NULL THEN to_account ELSE to_contract END) AS data
    FROM internal_transfers
    INNER JOIN calls ON internal_transfers.call_id = calls.id
    INNER JOIN contracts ON calls.contract_id = contracts.id
  UNION ALL
    SELECT
      contracts.id AS contract,
      'transfer_in' AS kind,
      internal_transfers.call_id AS call,
      internal_transfers.time,
      internal_transfers.msatoshi AS msatoshi,
      jsonb_build_object('other', CASE WHEN from_account IS NOT NULL THEN from_account ELSE from_contract END) AS data
    FROM internal_transfers
    INNER JOIN contracts ON internal_transfers.to_contract = contracts.id;
