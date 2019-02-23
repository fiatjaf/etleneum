import os
import shutil
import tempfile
import subprocess

import requests
import pytest
from lightning import LightningRpc
from bitcoin import BitcoinRPC

from .utils import TailableProc, wait_for, Contract

bitcoind_bin = os.getenv("BITCOIND")
lightningd_bin = os.getenv("LIGHTNINGD")
bitcoin_cli_bin = os.getenv("BITCOIN_CLI")


@pytest.fixture
def bitcoin_dir():
    bitcoin = tempfile.mkdtemp(prefix="bitcoin.")
    yield bitcoin
    shutil.rmtree(bitcoin)


@pytest.fixture
def lightning_dirs():
    lightning_a = tempfile.mkdtemp(prefix="lightning-a.")
    lightning_b = tempfile.mkdtemp(prefix="lightning-b.")
    lightning_c = tempfile.mkdtemp(prefix="lightning-c.")
    yield [lightning_a, lightning_b, lightning_c]
    shutil.rmtree(lightning_a)
    shutil.rmtree(lightning_b)
    shutil.rmtree(lightning_c)


@pytest.fixture
def bitcoind(bitcoin_dir):
    proc = TailableProc(
        "{bitcoind_bin} -regtest -datadir={dir} -server -printtoconsole -logtimestamps -nolisten -rpcport=10287 -rpcuser=rpcuser -rpcpassword=rpcpassword".format(
            bitcoind_bin=bitcoind_bin, dir=bitcoin_dir
        ),
        verbose=False,
        procname="bitcoind",
    )
    proc.start()
    proc.wait_for_log("Done loading")

    rpc = BitcoinRPC("http://127.0.0.1:10287/", "rpcuser", "rpcpassword")
    rpc.generate(101)

    yield proc, rpc

    proc.stop()


@pytest.fixture
def lightnings(bitcoin_dir, bitcoind, lightning_dirs):
    procs = []
    for i, dir in enumerate(lightning_dirs):
        proc = TailableProc(
            "{lightningd_bin} --network regtest --bitcoin-cli {bitcoin_cli_bin} --bitcoin-rpcport=10287 --bitcoin-datadir {bitcoin_dir} --bitcoin-rpcuser rpcuser --bitcoin-rpcpassword rpcpassword --lightning-dir {dir} --bind-addr 127.0.0.1:987{i}".format(
                lightningd_bin=lightningd_bin,
                bitcoin_cli_bin=bitcoin_cli_bin,
                bitcoin_dir=bitcoin_dir,
                dir=dir,
                i=i,
            ),
            verbose=False,
            procname="lightningd-{}".format(i),
        )
        proc.start()
        proc.wait_for_log("Server started with public key")
        procs.append(proc)

    # make rpc clients
    rpcs = []
    for dir in lightning_dirs:
        rpc = LightningRpc(os.path.join(dir, "lightning-rpc"))
        rpcs.append(rpc)

    # get nodes funded
    _, bitcoin_rpc = bitcoind
    for rpc in rpcs:
        addr = rpc.newaddr()["address"]
        bitcoin_rpc.sendtoaddress(addr, 15)
        bitcoin_rpc.generate(1)

    for rpc in rpcs:
        wait_for(lambda: len(rpc.listfunds()["outputs"]) == 1, timeout=60)

    # make a ring of channels
    for i in range(len(rpcs)):
        f = rpcs[(i + 1) % len(rpcs)]  # from
        t = rpcs[i]  # to
        tinfo = t.getinfo()
        f.connect(
            tinfo["id"], tinfo["binding"][0]["address"], tinfo["binding"][0]["port"]
        )
        num_tx = len(bitcoin_rpc.getrawmempool())
        f.fundchannel(tinfo["id"], 10000000)
        wait_for(lambda: len(bitcoin_rpc.getrawmempool()) == num_tx + 1)
        bitcoin_rpc.generate(1)

    # wait for channels
    for proc in procs:
        proc.wait_for_log("to CHANNELD_NORMAL", timeout=60)
    for rpc in rpcs:
        wait_for(lambda: len(rpc.listfunds()["channels"]) > 0, timeout=60)

    yield procs, rpcs

    # stop nodes
    for proc, rpc in zip(procs, rpcs):
        try:
            rpc.stop()
        except:
            pass

        proc.proc.wait(5)
        proc.stop()


@pytest.fixture
def init_db():
    db = os.getenv("DATABASE_URL")
    if "@localhost" not in db or "test" not in db:
        raise Exception("Use the test postgres database, please.")

    # destroy db
    end = subprocess.run(
        "psql {url} -c 'drop table contracts cascade; drop table calls;'".format(
            url=db
        ),
        shell=True,
        capture_output=True,
    )
    print("db destroy stdout: " + end.stdout.decode("utf-8"))
    print("db destroy stderr: " + end.stderr.decode("utf-8"))

    # rebuild db
    end = subprocess.run(
        "psql {url} -f postgres.sql".format(url=db), shell=True, capture_output=True
    )
    print("db creation stdout: " + end.stdout.decode("utf-8"))
    print("db creation stderr: " + end.stderr.decode("utf-8"))


@pytest.fixture
def flush_redis():
    r = os.getenv("REDIS_URL")
    if "localhost" not in r:
        raise Exception("Use the test redis database, please.")

    # delete everything
    end = subprocess.run("redis-cli flushdb", shell=True, capture_output=True)
    print("redis destroy stdout: " + end.stdout.decode("utf-8"))
    print("redis destroy stderr: " + end.stderr.decode("utf-8"))


@pytest.fixture
def etleneum(init_db, flush_redis, lightning_dirs, lightnings):
    dir_a = lightning_dirs[0]
    env = os.environ.copy()
    env.update({"SOCKET_PATH": os.path.join(dir_a, "lightning-rpc")})

    proc = TailableProc("./etleneum", env=env, procname="etleneum")
    proc.start()
    proc.wait_for_log("listening.")
    yield proc, env["SERVICE_URL"]

    proc.stop()


@pytest.fixture
def make_contract(etleneum, lightnings):
    etleneum_proc, url = etleneum
    _, [rpc_a, rpc_b, *_] = lightnings

    def make(**ctdata):
        r = requests.post(url + "/~/contract", json=ctdata)
        ctid = r.json()["value"]["id"]
        rpc_b.pay(r.json()["value"]["invoice"])
        rpc_a.waitinvoice("{}.{}".format(os.getenv("SERVICE_ID"), ctid))
        r = requests.post(url + "/~/contract/" + ctid)
        r.raise_for_status()
        return Contract(ctid, url, rpc_a, rpc_b, etleneum_proc)

    return make
