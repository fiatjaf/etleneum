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
  msatoshi numeric(13) NOT NULL DEFAULT 0,
  cost numeric(13) NOT NULL,
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
  msatoshi numeric(13) NOT NULL,
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
  msatoshi numeric(13) NOT NULL,
  fulfilled bool NOT NULL,
  bolt11 text NOT NULL
);

CREATE OR REPLACE FUNCTION funds(contract contracts) RETURNS bigint AS $$
  SELECT (
    SELECT coalesce(sum(msatoshi), 0)
    FROM calls WHERE calls.contract_id = contract.id
  ) - (
    SELECT coalesce(sum(msatoshi), 0)
    FROM internal_transfers WHERE from_contract = contract.id
  ) + (
    SELECT coalesce(sum(msatoshi), 0)
    FROM internal_transfers WHERE to_contract = contract.id
  );
$$ LANGUAGE SQL;

CREATE OR REPLACE FUNCTION balance(account accounts) RETURNS bigint AS $$
  SELECT (
    SELECT coalesce(sum(msatoshi), 0)
    FROM internal_transfers WHERE to_account = account.id
  ) - (
    SELECT coalesce(sum(msatoshi), 0)
    FROM internal_transfers WHERE from_account = account.id
  ) - (
    SELECT coalesce(sum(msatoshi), 0)
    FROM withdrawals WHERE account_id = account.id
  );
$$ LANGUAGE SQL;

CREATE OR REPLACE FUNCTION transfers(call text, contract text) RETURNS jsonb AS $$
  SELECT coalesce(to_jsonb(array_agg(transfers)), '[]'::jsonb)
  FROM
    (
      SELECT 'out' AS direction, msatoshi,
        CASE WHEN to_account IS NOT NULL THEN to_account ELSE to_contract END AS counterparty
      FROM internal_transfers
      WHERE internal_transfers.call_id = call
        AND internal_transfers.from_contract = contract
    UNION ALL
      SELECT 'in' AS direction, msatoshi,
        CASE WHEN from_account IS NOT NULL THEN from_account ELSE from_contract END AS counterparty
      FROM internal_transfers
      WHERE internal_transfers.call_id = call
        AND internal_transfers.to_contract = contract
    ) AS transfers
$$ LANGUAGE SQL;
