CREATE TABLE contract (
  id serial PRIMARY KEY,
  name text NOT NULL DEFAULT '',
  readme text NOT NULL DEFAULT '',
  code text,
  state jsonb NOT NULL DEFAULT '{}',

  CONSTRAINT state_is_object CHECK (jsonb_typeof(state) = 'object')
);

CREATE TABLE call (
  hash text UNIQUE NOT NULL, -- invoice hash
  label text UNIQUE NOT NULL, -- invoice label, unprefixed (xyz -> etlenum.xyz)
  time timestamp NOT NULL DEFAULT now(),
  contract_id int NOT NULL REFERENCES contract (id),
  method text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  satoshis int NOT NULL
);

table contract;
table call;
delete from call;
delete from contract;
