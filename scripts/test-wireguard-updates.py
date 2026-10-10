#!/usr/bin/env python3
"""Privileged, isolated regression for legacy CLI checks with a live kernel WARP outbound."""
import base64
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

core = os.environ["DUI_TEST_CORE"]
old_core = os.environ.get("DUI_TEST_OLD_CORE", core)
if "--isolated" not in sys.argv:
    # All interfaces, routes and sysctl writes in this test stay in a temporary namespace.
    raise SystemExit(subprocess.call(["unshare", "--net", sys.executable, __file__, "--isolated"]))

def command(args):
    return subprocess.check_output(args, text=True)

def network():
    return {
        "links": json.loads(command(["ip", "-j", "link", "show"])),
        "routes": json.loads(command(["ip", "-j", "-6", "route", "show", "table", "all"])),
        "rules": json.loads(command(["ip", "-j", "-6", "rule", "show"])),
        "sysctls": [Path(p).read_text() for p in (
            "/proc/sys/net/ipv4/conf/all/rp_filter",
            "/proc/sys/net/ipv6/conf/all/disable_ipv6")],
    }

key = base64.b64encode(bytes(range(1, 33))).decode()
config = {"log": {"loglevel": "none"}, "outbounds": [{
    "tag": "warp", "protocol": "wireguard",
    "settings": {
        "secretKey": key, "address": ["10.91.0.2/32", "fd91::2/128"],
        "noKernelTun": False,
        "peers": [{"publicKey": key, "endpoint": "127.0.0.1:59999"}],
    },
}]}

def launch(binary, path):
    proc = subprocess.Popen([binary, "run", "-config", str(path)],
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    return proc

def stop(proc):
    if proc is not None and proc.poll() is None:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()

with tempfile.TemporaryDirectory(prefix="dui-wireguard-update-") as directory:
    root = Path(directory)
    path = root / "config.json"
    payload = json.dumps(config).encode()
    path.write_bytes(payload)
    resident = launch(old_core, path)
    second = None
    try:
        for _ in range(100):
            if resident.poll() is not None:
                raise AssertionError("resident core did not start")
            if any(str(r.get("table")) == "10230" for r in network()["routes"]):
                break
            time.sleep(0.1)
        else:
            raise AssertionError("resident core did not create its kernel WireGuard table")
        time.sleep(1.2)  # Let IPv6 address initialization settle before snapshots.
        before = network()
        # These are the old updater's direct commands: no wrapper, environment flag,
        # namespace option or config rewrite is needed to validate the new binary.
        for args, data in (
            (["run", "-test", "-format", "json", "-config", "stdin:"], payload),
            (["-test", "-config", str(path)], None),
        ):
            checked = subprocess.run([core, *args], input=data, stdout=subprocess.PIPE,
                                     stderr=subprocess.STDOUT, timeout=30)
            assert checked.returncode == 0 and b"Configuration OK" in checked.stdout, "legacy config check failed"
            assert resident.poll() is None, "validation interrupted running core"
            assert network() == before, "validation changed kernel interfaces, routes, rules or sysctls"
            assert path.read_bytes() == payload, "validation changed existing configuration"

        userspace = copy.deepcopy(config)
        userspace["outbounds"][0]["settings"]["noKernelTun"] = True
        for variant in ({}, {"outbounds": [{"protocol": "freedom"}]}, userspace):
            checked = subprocess.run([core, "run", "-test", "-format", "json", "-config", "stdin:"],
                                     input=json.dumps(variant).encode(), stdout=subprocess.PIPE,
                                     stderr=subprocess.STDOUT, timeout=30)
            assert checked.returncode == 0, "no WireGuard / removed / userspace configuration check failed"
            assert resident.poll() is None and network() == before, "variant check changed the running network"

        invalid = copy.deepcopy(config)
        invalid["outbounds"][0]["settings"]["secretKey"] = "not-a-valid-key%"
        rejected = subprocess.run([core, "run", "-test", "-format", "json", "-config", "stdin:"],
                                  input=json.dumps(invalid).encode(), stdout=subprocess.PIPE,
                                  stderr=subprocess.STDOUT, timeout=30)
        assert rejected.returncode == 23, "invalid WireGuard config was accepted"
        assert resident.poll() is None and network() == before, "rejected check touched the running core"

        # Normal startup must retain the kernel choice and skip the occupied table.
        second = launch(core, path)
        for _ in range(100):
            assert second.poll() is None, "new normal startup collided with an occupied IPv6 table"
            if any(str(r.get("table")) == "10231" for r in network()["routes"]):
                break
            time.sleep(0.1)
        else:
            raise AssertionError("normal startup did not retain kernel WireGuard")
        print(json.dumps({
            "legacyStdinValidation": True, "withoutWireGuard": True, "removedWireGuard": True, "userspaceWireGuard": True, "legacyFileValidation": True,
            "savedConfigUnchanged": True, "runningCoreUninterrupted": True,
            "validationNetworkUnchanged": True, "invalidConfigRejected": True,
            "normalStartupUsesKernelTun": True, "occupiedTableSkipped": True,
        }))
    finally:
        stop(second)
        stop(resident)
