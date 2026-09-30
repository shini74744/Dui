"""Isolated Linux panel API regression. Never uses an installed database/config.

Set DUI_TEST_PANEL and DUI_TEST_CORE to the binaries being tested.
The temporary panel binds loopback and runs without root privileges.
"""
import http.cookiejar
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import sqlite3
import subprocess
import tempfile
import time
import urllib.parse
import urllib.request
import uuid


def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


uid = 65534 if os.geteuid() == 0 else os.getuid()
gid = 65534 if os.geteuid() == 0 else os.getgid()


def drop_root():
    if os.geteuid() == 0:
        os.setgroups([])
        os.setgid(gid)
        os.setuid(uid)


with tempfile.TemporaryDirectory(prefix='dui-closewait-api-') as tmp:
    root = Path(tmp)
    os.chown(root, uid, gid)
    for name in ('db', 'bin', 'logs'):
        (root/name).mkdir()
        os.chown(root/name, uid, gid)
    binary = root/'bin/xray-linux-amd64'
    for src, dst in [(os.environ['DUI_TEST_PANEL'], root/'x-ui'), (os.environ['DUI_TEST_CORE'], binary)]:
        shutil.copy2(src, dst)
        dst.chmod(0o755)
    env = os.environ.copy()
    env.update(XUI_DB_FOLDER=str(root/'db'), XUI_BIN_FOLDER=str(root/'bin'),
               XUI_LOG_FOLDER=str(root/'logs'), XUI_LOG_LEVEL='error')
    panel_port, api_port = port(), port()
    password = uuid.uuid4().hex
    result = subprocess.run([str(root/'x-ui'), 'setting', '-username', 'cw-test', '-password', password,
                             '-port', str(panel_port), '-webBasePath', '/', '-listenIP', '127.0.0.1'],
                            cwd=root, env=env, preexec_fn=drop_root, capture_output=True, timeout=30)
    assert result.returncode == 0, 'isolated database initialization failed'
    template = {
        'log': {'loglevel': 'none', 'access': 'none', 'error': 'none'},
        'api': {'tag': 'api', 'services': ['HandlerService', 'StatsService']}, 'stats': {},
        'inbounds': [{'tag': 'api', 'listen': '127.0.0.1', 'port': api_port, 'protocol': 'tunnel',
                      'settings': {'address': '127.0.0.1'}}],
        'outbounds': [{'tag': 'direct', 'protocol': 'freedom'}, {'tag': 'api', 'protocol': 'blackhole'}],
        'routing': {'rules': [{'type': 'field', 'inboundTag': ['api'], 'outboundTag': 'api'}]},
        'policy': {'levels': {'0': {'bufferSize': 64}}, 'system': {'statsInboundUplink': True}}}
    db = root/'db/x-ui.db'
    with sqlite3.connect(db) as conn:
        conn.execute('INSERT INTO settings(key,value) VALUES(?,?)', ('xrayTemplateConfig', json.dumps(template)))
    log = (root/'panel.log').open('w')
    proc = subprocess.Popen([str(root/'x-ui')], cwd=root, env=env, preexec_fn=drop_root,
                            start_new_session=True, stdout=log, stderr=log)
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    base = 'http://127.0.0.1:'+str(panel_port)

    def request(path, data):
        with opener.open(base+path, urllib.parse.urlencode(data).encode(), timeout=30) as r:
            return json.load(r)

    def saved():
        with sqlite3.connect(db) as conn:
            return json.loads(conn.execute("SELECT value FROM settings WHERE key='xrayTemplateConfig'").fetchone()[0])

    def status():
        r = request('/dui/xray/connection-policy/status', {})
        assert r['success'], r.get('msg')
        return r['obj']

    form = dict(handshake=4, connIdle=300, uplinkOnly=0, downlinkOnly=0, closeWaitTimeout=0)
    try:
        for _ in range(100):
            assert proc.poll() is None, 'isolated panel exited'
            try:
                with socket.create_connection(('127.0.0.1', panel_port), timeout=.1):
                    break
            except OSError:
                time.sleep(.1)
        assert request('/login', {'username': 'cw-test', 'password': password})['success']
        assert status()['closeWaitTimeout'] == 0
        for seconds in (1, 30, 600, 0):
            r = request('/dui/xray/connection-policy/apply', dict(form, closeWaitTimeout=seconds))
            assert r['success'], r.get('msg')
            assert status()['closeWaitTimeout'] == seconds
            for config in (saved(), json.loads((root/'bin/config.json').read_text())):
                assert config['policy']['system'].get('duiCloseWaitTimeout', 0) == seconds
                assert config['policy']['system']['statsInboundUplink'] is True
                assert config['policy']['levels']['0']['bufferSize'] == 64
        before = saved()
        for seconds in (-1, 601, '0.5'):
            assert not request('/dui/xray/connection-policy/apply', dict(form, closeWaitTimeout=seconds))['success']
            assert saved() == before
        # An old installed core must be rejected before saving/restarting.
        original = binary.with_suffix('.new-core')
        binary.rename(original)
        binary.write_text('#!/bin/sh\nif [ "$1" = "-version" ]; then echo "Xray vx-26.3 (Xray)"; exit 0; fi\nexit 19\n')
        binary.chmod(0o755)
        try:
            r = request('/dui/xray/connection-policy/apply', dict(form, closeWaitTimeout=30))
            assert not r['success'] and 'vx-26.4' in r['msg']
            assert saved() == before
        finally:
            binary.unlink()
            original.rename(binary)
        assert request('/dui/xray/connection-policy/apply', dict(form, closeWaitTimeout=30))['success']
        r = request('/dui/installXray/vx-26.3', {})
        assert not r['success'] and 'CLOSE-WAIT' in r['msg'], 'downgrade was not rejected before download/stop'
        with socket.create_connection(('127.0.0.1', api_port), timeout=2):
            pass
        with opener.open(base+'/dui/xray', timeout=20) as r:
            html = r.read().decode()
        assert 'CLOSE-WAIT 清理超时' in html and 'xrayPolicyForm.closeWaitTimeout' in html
        if os.environ.get('DUI_CAPTURE_HTML'):
            Path(os.environ['DUI_CAPTURE_HTML']).write_text(html)
        assert request('/dui/xray/connection-policy/apply', form)['success']
        assert 'duiCloseWaitTimeout' not in saved()['policy']['system']
        print(json.dumps({'api_save_reload_disable': True, 'runtime_policy_verified': True,
                          'invalid_values_preserve_database': True, 'old_core_rejected_before_save': True,
                          'downgrade_rejected_before_stop': True, 'rendered_setting_present': True}))
    finally:
        if proc.poll() is None:
            os.killpg(proc.pid, signal.SIGTERM)
            try:
                proc.wait(timeout=8)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.wait()
        log.close()
