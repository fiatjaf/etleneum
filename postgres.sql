CREATE TABLE contracts (
  id text PRIMARY KEY, -- prefix to turn into the init invoice label
  name text NOT NULL DEFAULT '',
  readme text NOT NULL DEFAULT '',
  code text NOT NULL,
  state jsonb NOT NULL DEFAULT '{}',
  funds int NOT NULL DEFAULT 0, -- total funds this contract can spend, in msats

  CONSTRAINT state_is_object CHECK (jsonb_typeof(state) = 'object'),
  CONSTRAINT code_exists CHECK (code != '')
);

CREATE TABLE calls (
  id text PRIMARY KEY, -- prefix to turn into invoice label
  hash text UNIQUE NOT NULL, -- invoice hash
  time timestamp NOT NULL DEFAULT now(),
  contract_id text NOT NULL REFERENCES contracts (id),
  method text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  satoshis int NOT NULL DEFAULT 0, -- total sats to be added to contracts funds
  cost int NOT NULL DEFAULT 0, -- cost of the call, in msats, paid to the platform

  CONSTRAINT method_exists CHECK (method != ''),
  CONSTRAINT hash_exists CHECK (hash != '')
);

table contracts;
table calls;
