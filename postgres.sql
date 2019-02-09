CREATE TABLE contracts (
  id text PRIMARY KEY, -- prefix to turn into the init invoice label
  name text NOT NULL DEFAULT '',
  readme text NOT NULL DEFAULT '',
  code text NOT NULL,
  state jsonb NOT NULL DEFAULT '{}',
  created_at timestamp NOT NULL DEFAULT now(),
  storage_costs int NOT NULL DEFAULT 0,

  CONSTRAINT state_is_object CHECK (jsonb_typeof(state) = 'object'),
  CONSTRAINT code_exists CHECK (code != '')
);

CREATE FUNCTION funds(contracts) RETURNS bigint AS $$
  SELECT 1 - $1.storage_costs + (
    SELECT coalesce(sum(1000*satoshis - paid), 0)
    FROM calls WHERE calls.contract_id = $1.id
  );
$$ LANGUAGE SQL;

CREATE TABLE calls (
  id text PRIMARY KEY,
  time timestamp NOT NULL DEFAULT now(),
  contract_id text NOT NULL REFERENCES contracts (id),
  method text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  satoshis int NOT NULL DEFAULT 0,
  cost int NOT NULL,
  paid int NOT NULL DEFAULT 0,

  CONSTRAINT method_exists CHECK (method != ''),
  CONSTRAINT cost_positive CHECK (CASE
    WHEN method == '__init__' THEN true
    ELSE cost > 0
  END),
  CONSTRAINT hash_exists CHECK (hash != '')
);



table contracts;
table calls;
