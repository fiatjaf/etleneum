import hashlib
import datetime
from pathlib import Path

import pytest


def test_http_time_hash(make_contract):
    contract = make_contract(
        name="test",
        readme="test test",
        code="""
function __init__ ()
  local resp = http.getjson("https://httpbin.org/anything?numbers=1&numbers=2&fruit=banana")
  return {
    args=resp.args,
    today=os.date("%Y-%m-%d", os.time()),
    hash=util.sha256("hash"),
  }
end
    """,
    )

    assert contract.get()["state"] == {
        "args": {"numbers": ["1", "2"], "fruit": "banana"},
        "today": datetime.date.today().isoformat(),
        "hash": hashlib.sha256(b"hash").hexdigest(),
    }


def test_pay_on_http_response(make_contract, lightnings):
    _, [_, rpc_b, rpc_c, *_] = lightnings

    contract = make_contract(
        name="test",
        readme="test test",
        code="""
function __init__ ()
  return {q=0, ended=false}
end

function deposit ()
  state.q = satoshis
  return {x=satoshis}
end

function withdraw ()
  local msatspaid = 0
  local x = http.getjson("https://httpbin.org/anything?x=" .. payload.x).args.x
  if tonumber(x) == state.q then
    local payee = "%s"
    msatspaid, err = ln.pay(payload.invoice, {payee=payee})
    if err == nil then
      state.q = 0
    end
    state.ended = true
  end
  return msatspaid
end
    """
        % rpc_c.getinfo()["id"],
    )

    ret = contract.call("deposit", {}, 23)
    assert contract.funds == 24000
    assert ret == {"x": 23}

    invalid_payee = rpc_b.invoice(
        label="invalid_payee", description="", msatoshi=24000
    )["bolt11"]
    invalid_amount = rpc_c.invoice(
        label="invalid_amount", description="", msatoshi=24001
    )["bolt11"]
    valid_invoice = rpc_c.invoice(label="valid", description="", msatoshi=23000)[
        "bolt11"
    ]

    # if condition doesn't match
    paid = contract.call("withdraw", {"x": 12, "invoice": valid_invoice}, 0)
    assert contract.state == {"ended": False, "q": 23}
    assert paid == 0
    assert contract.funds == 24000

    # condition matches, but payment fails
    paid = contract.call("withdraw", {"x": 23, "invoice": invalid_payee}, 0)
    assert contract.state == {"ended": True, "q": 23}
    assert paid == 0
    assert contract.funds == 24000

    # condition matches, but payments fails
    paid = contract.call("withdraw", {"x": 23, "invoice": invalid_amount}, 0)
    assert contract.state == {"ended": True, "q": 23}
    assert paid == 0
    assert contract.funds == 24000

    # payment succeeds
    paid = contract.call("withdraw", {"x": 23, "invoice": valid_invoice}, 0)
    assert contract.state == {"ended": True, "q": 0}
    assert paid == 23000
    assert contract.funds == 1000

    inv = rpc_c.waitinvoice("valid")
    assert inv["msatoshi_received"] == 23000


def test_sandbox(make_contract):
    Path("file").touch()

    with pytest.raises(Exception):
        contract = make_contract(
            name="test",
            readme="test test",
            code="""
    os.execute('rm file')
    os.execute('touch nofile')

    function __init__ ()
      os.execute("rm file")
      os.execute("touch nofile")
      return {x=os.getenv("HOME")}
    end
            """,
        )

    assert not Path("nofile").exists()
    assert Path("file").exists()
    Path("file").unlink()
    assert not Path("file").exists()

    with pytest.raises(Exception):
        assert contract.state == None


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


def test_hidden_fields(make_contract):
    contract = make_contract(
        name="test",
        readme="test test",
        code="""
function __init__ ()
  return {}
end

function silence ()
  state.message = payload._message
end
        """,
    )

    contract.call("silence", {"_message": "hello", "useless": "nada"}, 0)
    assert contract.calls[0]["method"] == "silence"
    assert contract.calls[0]["payload"] == {"useless": "nada"}


def test_global_methods_variables(make_contract):
    contract = make_contract(
        name="test",
        readme="test test",
        code="""
function __init__ ()
  return {}
end

local constant = 23

function fn ()
  return 23
end

function nothing ()
  return {constant=constant, fn=fn()}
end
        """,
    )

    ret = contract.call("nothing", {}, 0)
    assert ret == {"constant": 23, "fn": 23}
