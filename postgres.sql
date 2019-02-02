CREATE TABLE contracts (
  id text PRIMARY KEY, -- prefix to turn into the init invoice label
  name text NOT NULL DEFAULT '',
  readme text NOT NULL DEFAULT '',
  code text NOT NULL,
  state jsonb NOT NULL DEFAULT '{}',
  cost int NOT NULL, -- cost to create the contract in msats
  created_at timestamp NOT NULL DEFAULT now(),

  CONSTRAINT state_is_object CHECK (jsonb_typeof(state) = 'object'),
  CONSTRAINT cost_positive CHECK (cost > 0),
  CONSTRAINT code_exists CHECK (code != '')
);

CREATE TABLE calls (
  id text PRIMARY KEY, -- prefix to turn into invoice label
  time timestamp NOT NULL DEFAULT now(),
  contract_id text NOT NULL REFERENCES contracts (id),
  method text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  satoshis int NOT NULL DEFAULT 0, -- total sats to be added to contracts funds
  cost int NOT NULL, -- cost of the call, in msats, paid to the platform
  paid int NOT NULL DEFAULT 0, -- sum of payments, in sats, done by this call

  CONSTRAINT method_exists CHECK (method != ''),
  CONSTRAINT cost_positive CHECK (CASE
    WHEN method == '__init__' THEN true
    ELSE cost > 0
  END),
  CONSTRAINT hash_exists CHECK (hash != '')
);

table contracts;
table calls;
