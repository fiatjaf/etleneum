import os
import shutil
import tempfile
import subprocess

import pytest
from lightning import LightningRpc

from .utils import TailableProc


@pytest.fixture
def bitcoin_dir():
    bitcoin = tempfile.mkdtemp(prefix="bitcoin.")
    yield bitcoin
    shutil.rmtree(bitcoin)


@pytest.fixture
def lightning_dirs():
    lightning_a = tempfile.mkdtemp(prefix="lightning-a.")
    lightning_b = tempfile.mkdtemp(prefix="lightning-b.")
    yield [lightning_a, lightning_b]
    shutil.rmtree(lightning_a)
    shutil.rmtree(lightning_b)


@pytest.fixture
def bitcoind(bitcoin_dir):
    proc = TailableProc(
        "{bin} -regtest -datadir={dir} -server -printtoconsole -logtimestamps -nolisten -rpcport=10287".format(
            bin=os.getenv("BITCOIND"), dir=bitcoin_dir
        ),
        verbose=False,
        procname="bitcoind",
    )
    proc.start()
    proc.wait_for_log("Done loading")
    yield proc

    proc.stop()


@pytest.fixture
def lightnings(bitcoin_dir, bitcoind, lightning_dirs):
    lightningd_bin = os.getenv("LIGHTNINGD")
    bitcoin_cli_bin = os.getenv("BITCOIN_CLI")

    procs = []
    for i, dir in enumerate(lightning_dirs):
        proc = TailableProc(
            "{lightningd_bin} --network regtest --bitcoin-cli {bitcoin_cli_bin} --bitcoin-rpcport=10287 --bitcoin-datadir {bitcoin_dir} --lightning-dir {dir} --bind-addr 127.0.0.1:987{i}".format(
                lightningd_bin=lightningd_bin,
                bitcoin_cli_bin=bitcoin_cli_bin,
                bitcoin_dir=bitcoin_dir,
                dir=dir,
                i=i,
            ),
            procname="lightningd-{}".format(i),
        )
        proc.start()
        proc.wait_for_log("Server started with public key")
        procs.append(proc)

    yield procs

    for i, proc in enumerate(procs):
        try:
            rpc = LightningRpc(os.path.join(lightning_dirs[i], "lightning-rpc"))
            rpc.stop()
        except:
            pass

        proc.proc.wait(5)
        proc.stop()


@pytest.fixture
def init_db():
    db = os.getenv("DATABASE_URL")
    if "@localhost" not in db or "test" not in db:
        raise Exception("Use the test database, please.")

    end = subprocess.run(
        "psql {url} -f postgres.sql".format(url=db), shell=True, capture_output=True
    )
    print("db stdout: " + end.stdout.decode("utf-8"))
    print("db stderr: " + end.stderr.decode("utf-8"))


@pytest.fixture
def etleneum(init_db, lightning_dirs, lightnings):
    dir_a = lightning_dirs[0]
    env = os.environ.copy()
    env.update({"SOCKET_PATH": os.path.join(dir_a, "lightning-rpc")})

    proc = TailableProc("./etleneum", env=env, procname="etleneum")
    proc.start()
    proc.wait_for_log("listening.")
    yield env["SERVICE_URL"]

    proc.stop()
