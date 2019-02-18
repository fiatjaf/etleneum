import os
import requests


def test_contract_creation(etleneum, lightnings):
    etleneum_proc, url = etleneum
    _, [rpc_a, rpc_b, rpc_c, *_] = lightnings

    ctdata = {
        "name": "ico",
        "readme": "get rich",
        "code": """
function __init__ ()
  return {
    balances = {}
  }
end

function buy ()
  local price = 5

  local amount = payload.amount
  local user = payload.user

  if satoshis == amount * price then
    current = state.balances[user]
    if current == nil then
      current = 0
    end
    state.balances[user] = current + amount
  else
    error("wrong amount paid")
  end
end

function cashout ()
  local invoice = payload.invoice
  ln.pay(invoice)
end
        """,
    }

    # create contract
    r = requests.post(url + "/~/contract", json=ctdata)
    assert r.ok
    ctid = r.json()["value"]["id"]
    payment = rpc_b.pay(r.json()["value"]["invoice"])
    rpc_a.waitinvoice("{}.{}".format(os.getenv("SERVICE_ID"), ctid))
    r = requests.post(url + "/~/contract/" + ctid)
    assert r.ok

    # get contract calls (should contain the initial call)
    r = requests.get(url + "/~/contract/" + ctid + "/calls")
    assert r.ok
    assert len(r.json()["value"]) == 1
    assert set(r.json()["value"][0].items()) >= set(
        {
            "method": "__init__",
            "cost": payment["msatoshi"],
            "payload": {},
            "paid": 0,
            "satoshis": 1,
        }.items()
    )

    # prepare calls and send them
    current_state = {"balances": {}}
    current_funds = 1
    for buyer, amount, satoshis, succeed in [
        ("x", 2, 9, False),
        ("y", 30, 150, True),
        ("w", 123, 150, False),
        ("z", 0, 0, True),
        ("x", 2, 10, True),
    ]:
        r = requests.post(
            url + "/~/contract/" + ctid + "/call",
            json={
                "method": "buy",
                "payload": {"amount": amount, "user": buyer},
                "satoshis": satoshis,
            },
        )
        assert r.ok
        callid = r.json()["value"]["id"]
        cost = r.json()["value"]["cost"]
        assert cost > satoshis * 1000

        payment = rpc_b.pay(r.json()["value"]["invoice"])
        rpc_a.waitinvoice("{}.{}.{}".format(os.getenv("SERVICE_ID"), ctid, callid))
        assert payment["msatoshi"] == cost

        r = requests.post(url + "/~/call/" + callid)
        if succeed:
            assert r.ok
            bal = current_state["balances"].setdefault(buyer, 0)
            current_state["balances"][buyer] = bal + amount
            current_funds += satoshis
        else:
            assert not r.ok

        # check contract state after
        r = requests.get(url + "/~/contract/" + ctid)
        assert r.json()["value"]["state"] == current_state
        assert r.json()["value"]["funds"] == current_funds
        assert r.json()["value"]["paid"] == 0

    # get an invoice and fail to pay it with the contract
    inv = rpc_c.invoice(label="fail.1", desc="cashout", amount=current_funds + 1)
    bolt11 = inv["bolt11"]
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "cashout", "payload": {"invoice": bolt11}},
    )
    callid = r.json()["value"]["id"]
    cost = r.json()["value"]["cost"]
    payment = rpc_b.pay(r.json()["value"]["invoice"])
    rpc_a.waitinvoice("{}.{}.{}".format(os.getenv("SERVICE_ID"), ctid, callid))
    r = requests.post(url + "/~/call/" + callid)
    assert not r.ok
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.json()["value"]["funds"] == current_funds
    assert r.json()["value"]["paid"] == 0

    # succeed
    inv = rpc_c.invoice(label="succeed", desc="cashout", amount=current_funds)
    bolt11 = inv["bolt11"]
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "cashout", "payload": {"invoice": bolt11}},
    )
    callid = r.json()["value"]["id"]
    cost = r.json()["value"]["cost"]
    payment = rpc_b.pay(r.json()["value"]["invoice"])
    rpc_a.waitinvoice("{}.{}.{}".format(os.getenv("SERVICE_ID"), ctid, callid))
    r = requests.post(url + "/~/call/" + callid)
    assert r.ok
    rpc_c.waitinvoice("succeed")
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.json()["value"]["funds"] == 0
    assert r.json()["value"]["paid"] == current_funds
