import hashlib
import datetime
from pathlib import Path

import pytest


def test_http_time_hash_keybase(make_contract):
    contract = make_contract(
        name="test",
        readme="test test",
        code="""
function __init__ ()
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

  return {
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
    """,
    )

    state = contract.get()["state"]
    assert type(state["cuid"]) == str
    assert len(state["cuid"]) > 5
    del state["cuid"]

    assert state == {
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


def test_pay_on_http_response(make_contract, lightnings):
    _, [_, rpc_b, rpc_c, *_] = lightnings

    contract = make_contract(
        name="test",
        readme="test test",
        code="""
function __init__ ()
  return {deposited=0, conditionok=false, withdrawcalled=false, didwithdraw=false}
end

function deposit ()
  state.deposited = satoshis
  return {x=satoshis}
end

function withdraw ()
  local msatspaid = 0
  local x = http.getjson("https://httpbin.org/anything?x=" .. payload.x).args.x
  if tonumber(x) == state.deposited then
    local payee = "%s"
    msatspaid, err = ln.pay(payload.invoice, {payee=payee})
    if err == nil then
      state.didwithdraw = true
    end
    state.conditionok = true
  end
  state.withdrawcalled = true
  return msatspaid
end
    """
        % rpc_c.getinfo()["id"],
    )

    # starting state
    assert contract.state == {
        "deposited": 0,
        "withdrawcalled": False,
        "conditionok": False,
        "didwithdraw": False,
    }

    ret = contract.call("deposit", {}, 23)
    assert contract.funds == 23000
    assert ret == {"x": 23}
    assert contract.state == {
        "deposited": 23,
        "withdrawcalled": False,
        "conditionok": False,
        "didwithdraw": False,
    }

    invalid_payee = rpc_b.invoice(
        label="invalid_payee", description="", msatoshi=23000
    )["bolt11"]
    invalid_amount = rpc_c.invoice(
        label="invalid_amount", description="", msatoshi=23001
    )["bolt11"]
    valid_invoice = rpc_c.invoice(label="valid", description="", msatoshi=23000)[
        "bolt11"
    ]

    # condition matches, but payment fails due to lack of funds
    with pytest.raises(Exception):
        contract.call("withdraw", {"x": 23, "invoice": invalid_amount}, 0)
    assert contract.state == {
        "deposited": 23,
        "withdrawcalled": False,
        "conditionok": False,
        "didwithdraw": False,
    }
    assert contract.funds == 23000

    # if condition doesn't match
    paid = contract.call("withdraw", {"x": 12, "invoice": valid_invoice}, 0)
    assert contract.state == {
        "deposited": 23,
        "withdrawcalled": True,
        "conditionok": False,
        "didwithdraw": False,
    }
    assert paid == 0
    assert contract.funds == 23000

    # condition matches, but payments fails
    paid = contract.call("withdraw", {"x": 23, "invoice": invalid_payee}, 0)
    assert contract.state == {
        "deposited": 23,
        "withdrawcalled": True,
        "conditionok": True,
        "didwithdraw": False,
    }
    assert paid == 0
    assert contract.funds == 23000

    # payment succeeds
    paid = contract.call("withdraw", {"x": 23, "invoice": valid_invoice}, 0)
    assert contract.state == {
        "deposited": 23,
        "withdrawcalled": True,
        "conditionok": True,
        "didwithdraw": True,
    }
    assert paid == 23000
    assert contract.funds == 0

    inv = rpc_c.waitinvoice("valid")
    assert inv["msatoshi_received"] == 23000


def test_contract_funds_limit(make_contract, lightnings):
    _, [_, rpc_b, rpc_c, *_] = lightnings

    contract = make_contract(
        name="test",
        readme="test test",
        code="""
function __init__ ()
  return {}
end

function withdraw ()
  ln.pay(payload.invoice)
end
    """,
    )

    contract.refill(100)

    invoice = rpc_c.invoice(label="inv", description="", msatoshi=100000)["bolt11"]

    try:
        contract.call("withdraw", {"invoice": invoice}, 0)
        assert False
    except Exception:
        assert True


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


def test_timeout(make_contract):
    with pytest.raises(Exception):
        contract = make_contract(
            name="test",
            readme="test test",
            code="""
function __init__ ()
  while true do
    local x = 'y'
  end
end
            """,
        )

    with pytest.raises(Exception):
        assert contract.state == None


def test_syntax(make_contract):
    with pytest.raises(Exception):
        make_contract(
            name="test",
            readme="test test",
            code="""
function __init__
  return {}
end
            """,
        )

    with pytest.raises(Exception):
        make_contract(
            name="test",
            readme="test test",
            code="""
function start ()
  return {}
end
            """,
        )


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
