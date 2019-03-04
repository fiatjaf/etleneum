import os
import requests


def test_contract_calls(etleneum, lightnings):
    etleneum_proc, url = etleneum
    _, [rpc_a, rpc_b, rpc_c, *_] = lightnings

    ctdata = {
        "name": "ico",
        "readme": "get rich",
        "code": """
function __init__ ()
  return {
    balances = {dummy=0}
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
    call = r.json()["value"][0]
    assert call["method"] == "__init__"
    assert call["cost"] == payment["msatoshi"] - 1000  # -1 satoshi
    assert call["payload"] == {}
    assert call["paid"] == 0
    assert call["satoshis"] == 1

    # prepare calls and send them
    current_state = {"balances": {"dummy": 0}}
    current_funds = 1000
    current_call_n = 1
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
        assert (
            1100 > r.json()["value"]["cost"] > 1000
        )  # cost is 1000 + some small msats
        assert r.json()["value"]["satoshis"] == satoshis

        payment = rpc_b.pay(r.json()["value"]["invoice"])
        rpc_a.waitinvoice("{}.{}.{}".format(os.getenv("SERVICE_ID"), ctid, callid))
        assert (
            payment["msatoshi"]
            == r.json()["value"]["cost"] + 1000 * r.json()["value"]["satoshis"]
        )

        r = requests.post(url + "/~/call/" + callid)
        if succeed:
            assert r.ok
            bal = current_state["balances"].setdefault(buyer, 0)
            current_state["balances"][buyer] = bal + amount
            current_funds += satoshis * 1000
            current_call_n += 1
        else:
            assert not r.ok

        # check contract state and funds after
        r = requests.get(url + "/~/contract/" + ctid)
        assert (
            r.json()["value"]["state"]
            == current_state
            == requests.get(url + "/~/contract/" + ctid + "/state").json()["value"]
        )
        assert (
            r.json()["value"]["funds"]
            == current_funds
            == requests.get(url + "/~/contract/" + ctid + "/funds").json()["value"]
        )

        # calls after
        r = requests.get(url + "/~/contract/" + ctid + "/calls")
        assert r.ok
        assert len(r.json()["value"]) == current_call_n

    # get an invoice and fail to pay it with the contract
    inv = rpc_c.invoice(
        label="fail.1", description="cashout", msatoshi=current_funds + 1
    )
    bolt11 = inv["bolt11"]
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "cashout", "payload": {"invoice": bolt11}},
    )
    callid = r.json()["value"]["id"]
    payment = rpc_b.pay(r.json()["value"]["invoice"])
    rpc_a.waitinvoice("{}.{}.{}".format(os.getenv("SERVICE_ID"), ctid, callid))
    r = requests.post(url + "/~/call/" + callid)
    assert not r.ok  # contract without funds is always an error
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.json()["value"]["funds"] == current_funds

    # succeed
    inv = rpc_c.invoice(label="succeed", description="cashout", msatoshi=current_funds)
    bolt11 = inv["bolt11"]
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "cashout", "payload": {"invoice": bolt11}},
    )
    callid = r.json()["value"]["id"]
    payment = rpc_b.pay(r.json()["value"]["invoice"])
    rpc_a.waitinvoice("{}.{}.{}".format(os.getenv("SERVICE_ID"), ctid, callid))
    r = requests.post(url + "/~/call/" + callid)
    assert r.ok
    rpc_c.waitinvoice("succeed")
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.json()["value"]["funds"] == 0

    # calls after
    r = requests.get(url + "/~/contract/" + ctid + "/calls")
    assert r.ok
    assert len(r.json()["value"]) == current_call_n + 1
    assert r.json()["value"][0]["paid"] == current_funds
    assert r.json()["value"][0]["method"] == "cashout"

    # before should be the last buy that succeeded
    assert r.json()["value"][1]["paid"] == 0
    assert r.json()["value"][1]["method"] == "buy"
    assert r.json()["value"][1]["satoshis"] == 10
