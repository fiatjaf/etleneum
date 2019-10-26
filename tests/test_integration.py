import os
import json
import datetime
import urllib3
import hashlib
import subprocess
from pathlib import Path
from binascii import unhexlify

import requests
import sseclient


def test_contract_failure_and_refund(etleneum, lightnings):
    etleneum_proc, url = etleneum
    _, [rpc_a, rpc_b] = lightnings

    # there are zero contracts
    r = requests.get(url + "/~/contracts")
    assert r.ok
    assert r.json() == {"ok": True, "value": []}

    # try to create a contract with invalid lua code and fail
    r = requests.post(
        url + "/~/contract",
        json={
            "name": "qwe",
            "readme": "owwiawjhebqwljebqlejbqwelkjqwbelkqwbelqkwjbeqlwkjebqwlkjebqwlkjebqwlkjebqlk",
            "code": """
  function __init__
    return {}
  end
        """,
        },
    )

    # create a contract with valid lua, but then fail when executing __init__
    r = requests.post(
        url + "/~/contract",
        json={
            "name": "qwe",
            "readme": "owwiqwkjebqlwkjebqwebqwlkejbqwlkejbqwklejbqwklejbqwklebqwlkebqwklejbqwkljeb",
            "code": "function __init__ () return banana() end",
        },
    )
    assert r.ok
    ctid = r.json()["value"]["id"]
    bolt11 = r.json()["value"]["invoice"]

    # start listening for contract events
    sse = sseclient.SSEClient(
        urllib3.PoolManager().request(
            "GET", url + "/~~~/contract/" + ctid, preload_content=False
        )
    ).events()

    # pay
    payment = rpc_b.pay(bolt11)
    preimage = payment["payment_preimage"]

    # should run and fail
    assert next(sse).event == "call-run-event"
    ev = next(sse)
    assert ev.event == "contract-error"
    assert json.loads(ev.data)["id"] == ctid
    assert json.loads(ev.data)["kind"] == "runtime"

    # there are still zero contracts in the list
    r = requests.get(url + "/~/contracts")
    assert r.ok
    assert r.json() == {"ok": True, "value": []}

    # there's a pending refund
    r = requests.get(url + "/~/refunds")
    assert r.ok
    data = r.json()
    assert data["ok"]
    assert len(data["value"]) == 1

    refund = data["value"][0]
    assert refund["claimed"] == False
    assert refund["fulfilled"] == False
    assert (
        refund["msatoshi"]
        == payment["msatoshi"] - int(os.getenv("INITIAL_CONTRACT_COST_SATOSHIS")) * 1000
    )
    assert refund["hash"] == hashlib.sha256(unhexlify(preimage)).hexdigest()

    # withdraw refund
    r = requests.get(url + "/lnurl/refund?preimage=" + preimage)
    assert r.ok
    assert (
        r.json()["maxWithdrawable"] == r.json()["minWithdrawable"] == refund["msatoshi"]
    )
    bolt11 = rpc_b.invoice(
        refund["msatoshi"], "withdraw-refund", r.json()["defaultDescription"]
    )["bolt11"]
    r = requests.get(r.json()["callback"] + "?k1=" + preimage + "&pr=" + bolt11)
    assert r.json()["status"] == "OK"
    assert rpc_b.waitinvoice("withdraw-refund")["label"] == "withdraw-refund"


def test_everything_else(etleneum, lightnings):
    etleneum_proc, url = etleneum
    _, [rpc_a, rpc_b] = lightnings

    # there are zero contracts
    r = requests.get(url + "/~/contracts")
    assert r.ok
    assert r.json() == {"ok": True, "value": []}

    # create a valid contract
    ctdata = {
        "name": "ico",
        "readme": "get rich!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!",
        "code": """
function __init__ ()
  return {
    token_name='richcoin',
    balances={dummy=0}
  }
end

function setowner ()
  if not account.id then
    error('no account')
  end

  if account.get_balance() ~= 0 then
    error('this should never happen in this test')
  end

  contract.state.owner = account.id
end

function buy ()
  local price = 5000

  if call.msatoshi == call.payload.amount * price then
    current = contract.state.balances[call.payload.user]
    if current == nil then
      current = 0
    end
    contract.state.balances[call.payload.user] = current + call.payload.amount
  else
    error("wrong amount paid")
  end
end

function cashout () -- in which the contract owner takes all the funds and disappears
  if call.payload.amt_to_cashout < contract.get_funds() then
    error('you have to cashout all. tried to cashout ' .. call.payload.amt_to_cashout .. ' but contract has ' .. contract.get_funds())
  end

  contract.send(contract.state.owner, call.payload.amt_to_cashout)
end

function cashout_wrong ()
  contract.send(nil, call.payload.amt_to_cashout)
end

function return23 () return 23 end

local just23 = 23

function apitest ()
  local resp = http.getjson("https://httpbin.org/anything?numbers=1&numbers=2&fruit=banana")

  sig = [[
-----BEGIN PGP SIGNATURE-----
Version: Keybase OpenPGP v2.1.0
Comment: https://keybase.io/crypto

wsBcBAABCgAGBQJcqPiIAAoJEAJs7pbOl+xq3wAH/RdKQspEpZOFpRrurD21dlvj
2umI4Cu2XBOfVCNZPh++hpacNr2lk5iVvGm7eHgO54ybd11+b9QVcWwEyRLeKQhn
SbcPlc90POXZ05J3uwjVItLsNVW/Z9HYDDb8Fcf9C8s+ywVZ9oDHz9W4fRaBWKD3
Cwt2SscEsFFOTenlBJDU/8laX8EAzdqJ9PbUwqmwyrAYXmWqklLC7xOMdGHhLieZ
ZTlElCj5cSDz8M43sUMeGCzQ9v2MNxgz95GVZZwpTNI/Mut6d6d7UvQ/6bnvL65A
hG89/FmpOuSJgmxSoQgCsgPYuwqvcUpXx6sACJE1Zn4lyrDbi4zRH97cDKVhfjI=
=jha+
-----END PGP SIGNATURE-----
  ]]

  contract.state.apitest = {
    globalconstant=just23,
    globalfn=return23(),
    args=resp.args,
    today=os.date("%Y-%m-%d", os.time()),
    hash=util.sha256("hash"),
    kb_github=keybase.github("fiatjaf"),
    kb_domain=keybase.domain("fiatjaf.alhur.es"),
    sigok=keybase.verify("fiatjaf", "abc", sig),
    sigokt=keybase.verify(keybase.twitter("fiatjaf"), "abc", sig),
    signotok=keybase.verify("fiatjaf", "xyz", sig),
    signotokt=keybase.verify(keybase.twitter("qkwublakjbdaskjdb"), "abc", sig),
    userexists=keybase.exists('fiatjaf'),
    userdoesntexist=keybase.exists('qwkbqwelikqbeqw'),
    cuid=util.cuid()
  }
end

function losemoney ()
  -- do nothing, just eat the satoshis sent with the call
end

function infiniteloop ()
  while true do
    local x = 'y'
  end
end

function dangerous ()
  os.execute("rm file")
  os.execute("touch nofile")
  contract.state.home = os.getenv("HOME")
end
        """,
    }

    #  prepare contract
    r = requests.post(url + "/~/contract", json=ctdata)
    assert r.ok
    ctid = r.json()["value"]["id"]
    bolt11 = r.json()["value"]["invoice"]

    sse = sseclient.SSEClient(
        urllib3.PoolManager().request(
            "GET", url + "/~~~/contract/" + ctid, preload_content=False
        )
    ).events()

    # there are still zero contracts in the list
    r = requests.get(url + "/~/contracts")
    assert r.ok
    assert r.json() == {"ok": True, "value": []}

    # get prepared
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.ok
    assert r.json()["value"]["invoice"] == bolt11
    assert r.json()["value"]["code"] == ctdata["code"]
    assert r.json()["value"]["name"] == ctdata["name"]
    assert r.json()["value"]["readme"] == ctdata["readme"]
    assert r.json()["value"]["invoice_paid"] == False

    # pay for contract
    payment = rpc_b.pay(bolt11)

    # it should get created and we should get a notification
    assert next(sse).event == "call-run-event"
    ev = next(sse)
    assert ev.event == "contract-created"
    assert json.loads(ev.data)["id"] == ctid

    # check contract info
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.ok
    assert "invoice" not in r.json()["value"]
    assert r.json()["value"]["code"] == ctdata["code"]
    assert r.json()["value"]["name"] == ctdata["name"]
    assert r.json()["value"]["readme"] == ctdata["readme"]
    assert r.json()["value"]["state"]["token_name"] == "richcoin"
    assert r.json()["value"]["funds"] == 0

    # contract list should show this single contract
    r = requests.get(url + "/~/contracts")
    assert r.ok
    contracts = r.json()["value"]
    assert len(contracts) == 1
    assert contracts[0]["name"] == ctdata["name"]
    assert contracts[0]["readme"] == ctdata["readme"]
    assert contracts[0]["id"] == ctid
    assert contracts[0]["funds"] == 0
    assert contracts[0]["ncalls"] == 1

    # get contract calls (should contain the initial call)
    r = requests.get(url + "/~/contract/" + ctid + "/calls")
    assert r.ok
    assert len(r.json()["value"]) == 1
    call = r.json()["value"][0]
    assert call["method"] == "__init__"
    assert call["cost"] == payment["msatoshi"]
    assert call["payload"] == {}
    assert call["msatoshi"] == 0

    # prepare a call and then patch it, but then ignore it
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={
            "method": "buy",
            "payload": {"amount": 1, "user": "ttt", "x": "t"},
            "msatoshi": 10000,
        },
    )
    assert r.ok
    callid = r.json()["value"]["id"]
    r = requests.patch(url + "/~/call/" + callid, json={"amount": 2, "user": "uuu"})
    assert r.ok
    r = requests.get(url + "/~/call/" + callid)
    assert r.ok
    assert r.json()["value"]["id"] == callid
    assert r.json()["value"]["payload"] == {"amount": 2, "user": "uuu", "x": "t"}
    assert r.json()["value"]["msatoshi"] == 10000

    # set contract owner in a very insecure way
    ## fail because no logged account
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "setowner", "payload": {}},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-error"

    ## create a fake session, then succeed
    subprocess.run("redis-cli setex auth-session:zxcasdqwe 999 account1", shell=True)
    r = requests.post(
        url + "/~/contract/" + ctid + "/call?session=zxcasdqwe",
        json={"method": "setowner", "payload": {}},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-made"

    ## fail because no logged account
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "setowner", "payload": {}},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-error"

    # prepare calls and send them
    current_state = {
        "balances": {"dummy": 0},
        "token_name": "richcoin",
        "owner": "account1",
    }
    current_funds = 0
    current_call_n = 2  # __ini__ and setowner
    for buyer, amount, msatoshi, succeed in [
        ("x", 2, 9000, False),
        ("x", 0, 0, True),
        ("y", 2, 10000, True),
    ]:
        r = requests.post(
            url + "/~/contract/" + ctid + "/call",
            json={
                "method": "buy",
                "payload": {"amount": amount, "user": buyer},
                "msatoshi": msatoshi,
            },
        )
        assert r.ok
        callid = r.json()["value"]["id"]
        assert 6000 > r.json()["value"]["cost"] > 1000
        assert r.json()["value"]["msatoshi"] == msatoshi

        payment = rpc_b.pay(r.json()["value"]["invoice"])
        assert (
            payment["msatoshi"]
            == r.json()["value"]["cost"] + r.json()["value"]["msatoshi"]
        )

        assert next(sse).event == "call-run-event"
        ev = next(sse)
        assert json.loads(ev.data)["id"] == callid

        if succeed:
            assert ev.event == "call-made"
            bal = current_state["balances"].setdefault(buyer, 0)
            current_state["balances"][buyer] = bal + amount
            current_funds += msatoshi
            current_call_n += 1
        else:
            assert ev.event == "call-error"

            # perform a refund
            r = requests.get(url + "/~/refunds")
            data = r.json()
            assert len(data["value"]) == 1
            refund = data["value"][0]
            assert refund["claimed"] == False
            assert refund["fulfilled"] == False
            assert refund["msatoshi"] == msatoshi
            assert refund["hash"] == payment["payment_hash"]
            r = requests.get(
                url + "/lnurl/refund?preimage=" + payment["payment_preimage"]
            )
            assert r.ok
            assert (
                r.json()["maxWithdrawable"]
                == r.json()["minWithdrawable"]
                == refund["msatoshi"]
            )
            bolt11 = rpc_b.invoice(
                refund["msatoshi"],
                "withdraw-refund-" + callid,
                r.json()["defaultDescription"],
            )["bolt11"]
            r = requests.get(
                r.json()["callback"]
                + "?k1="
                + payment["payment_preimage"]
                + "&pr="
                + bolt11
            )
            assert r.json()["status"] == "OK"
            assert (
                rpc_b.waitinvoice("withdraw-refund-" + callid)["label"]
                == "withdraw-refund-" + callid
            )

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

    # try to cash out to our own scammer balance
    ## fail because of too big amount
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "cashout", "payload": {"amt_to_cashout": current_funds + 1}},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-error"
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.json()["value"]["funds"] == current_funds

    ## also fail because of too small amount
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "cashout", "payload": {"amt_to_cashout": current_funds - 1}},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-error"
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.json()["value"]["funds"] == current_funds

    ## fail calling a buggy version of the same method that sends to nil
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "cashout_wrong", "payload": {"amt_to_cashout": current_funds}},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-error"
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.json()["value"]["funds"] == current_funds

    ## then succeed
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "cashout", "payload": {"amt_to_cashout": current_funds}},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-made"
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.json()["value"]["funds"] == 0

    # calls after
    r = requests.get(url + "/~/contract/" + ctid + "/calls")
    assert r.ok
    assert len(r.json()["value"]) == current_call_n + 1
    assert r.json()["value"][0]["method"] == "cashout"

    # before should be the last buy that succeeded
    assert r.json()["value"][1]["method"] == "buy"
    assert r.json()["value"][1]["msatoshi"] == 10000

    # call a method that will timeout
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "infiniteloop", "payload": {}},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    ev = next(sse)
    assert ev.event == "call-error"
    assert json.loads(ev.data)["kind"] == "runtime"

    # call a method that should break out of the sandbox
    Path("file").touch()
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "dangerous", "payload": {}},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    ev = next(sse)
    assert ev.event == "call-error"
    assert json.loads(ev.data)["kind"] == "runtime"
    assert (
        "home"
        not in requests.get(url + "/~/contract/" + ctid + "/state").json()["value"]
    )
    assert not Path("nofile").exists()
    assert Path("file").exists()
    Path("file").unlink()
    assert not Path("file").exists()

    # call the method that tests all the fancy lua apis
    r = requests.post(
        url + "/~/contract/" + ctid + "/call", json={"method": "apitest", "payload": {}}
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-made"

    data = requests.get(url + "/~/contract/" + ctid + "/state").json()["value"][
        "apitest"
    ]
    assert type(data["cuid"]) == str
    assert len(data["cuid"]) > 5
    del data["cuid"]

    assert data == {
        "globalfn": 23,
        "globalconstant": 23,
        "args": {"numbers": ["1", "2"], "fruit": "banana"},
        "today": datetime.date.today().isoformat(),
        "hash": hashlib.sha256(b"hash").hexdigest(),
        "kb_github": "fiatjaf",
        "kb_domain": "fiatjaf",
        "sigok": True,
        "sigokt": True,
        "signotok": False,
        "signotokt": False,
        "userexists": True,
        "userdoesntexist": False,
    }

    # send a lot of money to the contract so we can have incoming capacity in our second node for the next step
    r = requests.post(
        url + "/~/contract/" + ctid + "/call",
        json={"method": "losemoney", "payload": {}, "msatoshi": 444444444},
    )
    rpc_b.pay(r.json()["value"]["invoice"])
    assert next(sse).event == "call-run-event"
    assert next(sse).event == "call-made"

    # finally withdraw our scammer balance
    r = requests.get(url + "/lnurl/withdraw?session=zxcasdqwe")
    assert r.ok
    assert r.json()["maxWithdrawable"] == current_funds
    bolt11 = rpc_b.invoice(
        current_funds, "withdraw-scam", r.json()["defaultDescription"]
    )["bolt11"]
    r = requests.get(r.json()["callback"] + "?k1=zxcasdqwe&pr=" + bolt11)
    assert rpc_b.waitinvoice("withdraw-scam")["label"] == "withdraw-scam"
