import requests


def test_retry_payment(make_contract, lightnings, etleneum):
    _, [_, rpc_b, rpc_c, *_] = lightnings
    etleneum_proc, url = etleneum

    contract = make_contract(
        name="test",
        readme="test test",
        code="""
function __init__ ()
  return {x=1}
end

function withdraw ()
  ln.pay(payload._invoice)
end
    """,
    )

    contract.refill(10)

    # should fail because of no routes
    old = rpc_b.invoice(label="0", description="", msatoshi=10000)["bolt11"]
    contract.call("withdraw", {"_invoice": old}, 0)
    etleneum_proc.wait_for_log("payment failed", timeout=15)

    # retry payment with invalid amount
    new = rpc_c.invoice(label="1", description="", msatoshi=20000)["bolt11"]
    r = requests.post(url + "/~/retry/" + old, json={"invoice": new})
    assert not r.ok

    # retry payment with invalid old reference
    new = rpc_c.invoice(label="3", description="", msatoshi=10000)["bolt11"]
    r = requests.post(url + "/~/retry/" + old + "wrong", json={"invoice": new})
    assert not r.ok

    # retry with a valid amount and correct reference
    r = requests.post(url + "/~/retry/" + old, json={"invoice": new})
    assert r.ok
    etleneum_proc.wait_for_log("payment succeeded", timeout=15)
