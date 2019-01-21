CREATE TABLE contract (
  id serial PRIMARY KEY, -- turned into a hashid and then into the init invoice label
  name text NOT NULL DEFAULT '',
  readme text NOT NULL DEFAULT '',
  code text NOT NULL,
  state jsonb NOT NULL DEFAULT '{}',
  funds int NOT NULL DEFAULT 0, -- total funds this contract can spend, in msats

  CONSTRAINT state_is_object CHECK (jsonb_typeof(state) = 'object'),
  CONSTRAINT code_exists CHECK (code != ''),
);

CREATE TABLE call (
  hash text UNIQUE NOT NULL, -- invoice hash
  label text UNIQUE NOT NULL, -- invoice label
  time timestamp NOT NULL DEFAULT now(),
  contract_id int NOT NULL REFERENCES contract (id),
  method text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  satoshis int NOT NULL DEFAULT 0, -- total sats to be added to contracts funds
  cost int NOT NULL DEFAULT 0, -- cost of the call, in msats, paid to the platform

  CONSTRAINT method_exists CHECK (method != ''),
  CONSTRAINT label_exists CHECK (label != ''),
  CONSTRAINT hash_exists CHECK (hash != '')
);

table contract;
table call;
