import re
import os
import time
import shlex
import logging
import subprocess
import threading

import requests

TIMEOUT = 5


def wait_for(success, timeout=TIMEOUT):
    start_time = time.time()
    interval = 0.25
    while not success() and time.time() < start_time + timeout:
        time.sleep(interval)
        interval *= 2
        if interval > 5:
            interval = 5
    if time.time() > start_time + timeout:
        raise ValueError("Error waiting for {}", success)


class TailableProc(object):
    """A monitorable process that we can start, stop and tail.
    This is the base class for the daemons. It allows us to directly
    tail the processes and react to their output.
    """

    def __init__(self, cmd_line, env=None, outputDir=None, verbose=True, procname=None):
        self.cmd_line = cmd_line
        self.logs = []
        self.logs_cond = threading.Condition(threading.RLock())
        self.env = env or os.environ.copy()
        self.running = False
        self.proc = None
        self.outputDir = outputDir
        self.logsearch_start = 0
        self.procname = procname or ""

        # Should we be logging lines we read from stdout?
        self.verbose = verbose

        # A filter function that'll tell us whether to filter out the line (not
        # pass it to the log matcher and not print it to stdout).
        self.log_filter = lambda line: False

    def start(self):
        """Start the underlying process and start monitoring it.
        """
        logging.debug("Starting '%s'" % self.cmd_line)
        self.proc = subprocess.Popen(
            shlex.split(self.cmd_line),
            stderr=subprocess.STDOUT,
            stdout=subprocess.PIPE,
            env=self.env,
        )
        self.thread = threading.Thread(target=self.tail)
        self.thread.daemon = True
        self.thread.start()
        self.running = True

    def stop(self, timeout=10):
        self.proc.terminate()

        # Now give it some time to react to the signal
        rc = self.proc.wait(timeout)

        if rc is None:
            self.proc.kill()

        self.proc.wait()
        self.thread.join(timeout=TIMEOUT)

        if self.proc.returncode:
            raise ValueError(
                "Process '{} ({})' did not cleanly shutdown: return code {}".format(
                    self.procname, self.proc.pid, rc
                )
            )

        return self.proc.returncode

    def kill(self):
        """Kill process without giving it warning."""
        self.proc.kill()
        self.proc.wait()
        self.thread.join(timeout=TIMEOUT)

    def tail(self):
        """Tail the stdout of the process and remember it.
        Stores the lines of output produced by the process in
        self.logs and signals that a new line was read so that it can
        be picked up by consumers.
        """
        for line in iter(self.proc.stdout.readline, ""):
            if len(line) == 0:
                break
            if self.log_filter(line.decode("ASCII")):
                continue
            if self.verbose:
                logging.debug("%s: %s", self.procname, line.decode().rstrip())
            with self.logs_cond:
                self.logs.append(str(line.rstrip()))
                self.logs_cond.notifyAll()
        self.running = False
        self.proc.stdout.close()

    def is_in_log(self, regex, start=0):
        """Look for `regex` in the logs."""

        ex = re.compile(regex)
        for l in self.logs[start:]:
            if ex.search(l):
                logging.debug("Found '%s' in logs", regex)
                return l

        logging.debug("Did not find '%s' in logs", regex)
        return None

    def wait_for_logs(self, regexs, timeout=TIMEOUT):
        """Look for `regexs` in the logs.
        We tail the stdout of the process and look for each regex in `regexs`,
        starting from last of the previous waited-for log entries (if any).  We
        fail if the timeout is exceeded or if the underlying process
        exits before all the `regexs` were found.
        If timeout is None, no time-out is applied.
        """
        logging.debug("Waiting for {} in the logs".format(regexs))
        exs = [re.compile(r) for r in regexs]
        start_time = time.time()
        pos = self.logsearch_start
        while True:
            if timeout is not None and time.time() > start_time + timeout:
                print("Time-out: can't find {} in logs".format(exs))
                for r in exs:
                    if self.is_in_log(r):
                        print("({} was previously in logs!)".format(r))
                raise TimeoutError('Unable to find "{}" in logs.'.format(exs))
            elif not self.running:
                raise ValueError("Process died while waiting for logs.")

            with self.logs_cond:
                if pos >= len(self.logs):
                    self.logs_cond.wait(1)
                    continue

                for r in exs.copy():
                    self.logsearch_start = pos + 1
                    if r.search(self.logs[pos]):
                        logging.debug("Found '%s' in logs", r)
                        exs.remove(r)
                        break
                if len(exs) == 0:
                    return self.logs[pos]
                pos += 1

    def wait_for_log(self, regex, timeout=TIMEOUT):
        """Look for `regex` in the logs.
        Convenience wrapper for the common case of only seeking a single entry.
        """
        return self.wait_for_logs([regex], timeout)


class Contract(object):
    def __init__(self, id, url, payee_rpc, payer_rpc, etleneum_proc):
        self.id = id
        self.url = url
        self.payee_rpc = payee_rpc
        self.payer_rpc = payer_rpc
        self.etleneum_proc = etleneum_proc

    def get(self):
        r = requests.get(self.url + "/~/contract/" + self.id)
        r.raise_for_status()
        return r.json()["value"]

    @property
    def funds(self):
        r = requests.get(self.url + "/~/contract/" + self.id + "/funds")
        r.raise_for_status()
        return r.json()["value"]

    @property
    def state(self):
        r = requests.get(self.url + "/~/contract/" + self.id + "/state")
        r.raise_for_status()
        return r.json()["value"]

    @property
    def calls(self):
        r = requests.get(self.url + "/~/contract/" + self.id + "/calls")
        r.raise_for_status()
        return r.json()["value"]

    def refill(self, satoshis):
        r = requests.get(
            self.url + "/~/contract/" + self.id + "/refill/" + str(satoshis)
        )
        r.raise_for_status()
        self.payer_rpc.pay(r.json()["value"])
        self.etleneum_proc.wait_for_log("contract refill")

    def call(self, method, payload, satoshis):
        r = requests.post(
            self.url + "/~/contract/" + self.id + "/call",
            json={"method": method, "payload": payload, "satoshis": satoshis},
        )
        callid = r.json()["value"]["id"]
        self.payer_rpc.pay(r.json()["value"]["invoice"])
        self.payee_rpc.waitinvoice(
            "{}.{}.{}".format(os.getenv("SERVICE_ID"), self.id, callid)
        )

        r = requests.post(self.url + "/~/call/" + callid)
        r.raise_for_status()
        return r.json()["value"]
