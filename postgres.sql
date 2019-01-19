CREATE TABLE contract (
  id serial PRIMARY KEY,
  code text,
  state jsonb NOT NULL DEFAULT '{}',

  CONSTRAINT state_is_object CHECK (jsonb_typeof(state) = 'object')
);

CREATE TABLE call (
  hash text UNIQUE NOT NULL, -- invoice hash
  label text UNIQUE NOT NULL, -- invoice label
  time timestamp NOT NULL DEFAULT now(),
  contract_id int NOT NULL REFERENCES contract (id),
  method text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  satoshis int NOT NULL
);

table contract;
table call;

update contract set code = '
price = 5

function buytoken ()
  local amount = payload["amount"]
  local user = payload["user"]

  if satoshis == amount * price then
    current = state[user]
    if current == nil then
      current = 0
    end
    state[user] = current + amount
  end

  for k, v in pairs(state) do
    print(k, v)
  end
end

function withdrawfunds ()
  ln.pay(payload["bolt11"])
end
' where id = 1;
