function __init__ ()
  return {
    name="my ico",
    symbol="%",
    totalSupply=0,
    hashes={},
    balances={}
  }
end

function buytoken ()
  local price = 5

  local amount = payload.amount
  local user = payload.user
  local initialhash = payload.initialhash

  -- check if user exists
  -- if not it must be created with a hash
  if state.hashes[user] == nil then
    if initialhash ~= nil then
      state.hashes[user] = initialhash
    else
      error("missing initial hash to create this user")
    end
  end

  if satoshis == amount * price then
    current = state.balances[user]
    if current == nil then
      current = 0
    end
    state.balances[user] = current + amount
    state.totalSupply = state.totalSupply + amount
  else
    error("insufficient payment")
  end
end

function transfer()
  local from = payload.from
  local preimage = payload.preimage
  local nexthash = payload.nexthash
  local to = payload.to
  local amount = payload.amount

  local userhash = state.hashes[from]
  if userhash == nil then
    error("sending user doesn't exist")
  end

  if sha256(preimage) == userhash then
    -- authorized to transfer funds from this user
    state.hashes[from] = nexthash
  else
    error("unauthorized")
  end

  local bfrom = state.balances[from]
  if bfrom == nil then
    bfrom = 0
  end

  if bfrom < amount then
    error("insufficient balances")
  end

  local bto = state.balances[to]
  if bto == nil then
    error("receiving user doesn't exist")
  end

  state.balances[from] = bfrom - amount
  state.balances[to] = bto + amount

  return true
end

function withdrawfunds ()
  local preimage = payload.preimage
  local nexthash = payload.nexthash
  local invoice = payload.invoice

  if sha256(preimage) == state.masterhash then
    ln.pay(invoice)
    state.masterhash = nexthash
  else
    error("unauthorized")
  end
end
