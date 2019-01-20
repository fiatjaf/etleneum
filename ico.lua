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
