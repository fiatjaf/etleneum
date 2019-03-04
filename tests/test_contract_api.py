import os
import requests


def test_contract_creation(etleneum, lightnings):
    etleneum_proc, url = etleneum
    _, [rpc_a, rpc_b, *_] = lightnings

    # there are zero contracts
    r = requests.get(url + "/~/contracts")
    assert r.ok
    assert r.json() == {"ok": True, "value": []}

    ctdata = {
        "code": "function __init__()\n  return {x=23}\nend",
        "name": "test",
        "readme": "readme",
    }

    # prepare contract
    r = requests.post(url + "/~/contract", json=ctdata)
    assert r.ok
    bolt11 = r.json()["value"]["invoice"]
    assert bolt11.startswith("lnbc")
    ctid = r.json()["value"]["id"]

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

    # enable contract should fail before invoice is paid
    r = requests.post(url + "/~/contract/" + ctid)
    assert not r.ok
    assert r.status_code == 402

    # pay invoice
    rpc_b.pay(bolt11)
    rpc_a.waitinvoice("{}.{}".format(os.getenv("SERVICE_ID"), ctid))

    # should say invoice_paid
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.ok
    assert r.json()["value"]["invoice_paid"] == True

    # enable contract
    r = requests.post(url + "/~/contract/" + ctid)
    assert r.ok
    assert r.json()["ok"] == True

    # get contract info
    r = requests.get(url + "/~/contract/" + ctid)
    assert r.ok
    assert "invoice" not in r.json()["value"]
    assert r.json()["value"]["code"] == ctdata["code"]
    assert r.json()["value"]["name"] == ctdata["name"]
    assert r.json()["value"]["readme"] == ctdata["readme"]
    assert r.json()["value"]["state"] == {"x": 23}
    assert r.json()["value"]["funds"] == 1000
    assert r.json()["value"]["refilled"] == 0
    assert r.json()["value"]["storage_costs"] == 0

    # contract list should show this
    r = requests.get(url + "/~/contracts")
    assert r.ok
    contracts = r.json()["value"]
    assert len(contracts) == 1
    assert contracts[0]["name"] == ctdata["name"]
    assert contracts[0]["readme"] == ctdata["readme"]
    assert contracts[0]["id"] == ctid
    assert contracts[0]["funds"] == 1000
    assert contracts[0]["ncalls"] == 1

    # get contract calls (should contain the initial call)
    r = requests.get(url + "/~/contract/" + ctid + "/calls")
    assert r.ok
    assert len(r.json()["value"]) == 1

    # refill contract
    r = requests.get(url + "/~/contract/" + ctid + "/refill/18")
    assert r.ok
    assert r.json()["value"].startswith("lnbc")

    rpc_b.pay(r.json()["value"])
    etleneum_proc.wait_for_log("got payment")

    r = requests.get(url + "/~/contract/" + ctid)
    assert r.ok
    assert r.json()["value"]["code"] == ctdata["code"]
    assert r.json()["value"]["name"] == ctdata["name"]
    assert r.json()["value"]["readme"] == ctdata["readme"]
    assert r.json()["value"]["state"] == {"x": 23}
    assert r.json()["value"]["refilled"] == 18000
    assert r.json()["value"]["funds"] == 19000
    assert r.json()["value"]["storage_costs"] == 0


def test_refill(make_contract):
    contract = make_contract(
        name="test",
        readme="test test",
        code="""
function __init__ ()
  return {x=1}
end
        """,
    )

    contract.refill(10)
    assert contract.funds == 11000
