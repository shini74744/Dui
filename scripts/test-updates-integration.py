"""Isolated verified update API regression. Never uses an installed database/config.

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


with tempfile.TemporaryDirectory(prefix='dui-strategy-api-') as tmp:
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

    try:
        for _ in range(100):
            assert proc.poll() is None, 'isolated panel exited'
            try:
                with socket.create_connection(('127.0.0.1',panel_port),timeout=.1):break
            except OSError:time.sleep(.1)
        # Both root aliases and /dui/api routes must require authentication.
        for path in ('/updates','/dui/api/server/updates'):
            with opener.open(base+path,timeout=10) as response:
                content=response.read()
                assert b'"supported"' not in content and b'"items"' not in content
        for path in ('/updates/start','/dui/api/server/updates/start','/installXray/vx-26.7'):
            req=urllib.request.Request(base+path,data=b'{"kind":"panel","version":"v99.1.1"}',headers={'Content-Type':'application/json'})
            try:
                with opener.open(req,timeout=10) as response:
                    assert b'"id"' not in response.read()
            except urllib.error.HTTPError as err:assert err.code in (302,303,307,401,403)
        assert not (root/'db/updates/job.json').exists()
        assert request('/login',{'username':'cw-test','password':password})['success']
        with opener.open(base+'/dui/api/server/updates',timeout=10) as response:
            data=json.load(response)
        assert data['success'] and not data['obj']['supported']
        assert set(data['obj']['items'])=={'panel','core'}
        body=b'{"kind":"panel","version":"v99.1.1"}'
        try:
            req=urllib.request.Request(base+'/dui/api/server/updates/start',data=body,headers={'Content-Type':'application/json','Origin':'https://untrusted.example'})
            opener.open(req,timeout=10)
            raise AssertionError('cross-origin request accepted')
        except urllib.error.HTTPError as err:assert err.code==403
        req=urllib.request.Request(base+'/dui/api/server/updates/start',data=body,headers={'Content-Type':'application/json'})
        with opener.open(req,timeout=10) as response:data=json.load(response)
        assert not data['success'] and 'unsupported_installation' in data['msg'],data
        assert not (root/'db/updates/job.json').exists()
        with opener.open(base+'/dui/',timeout=10) as response:
            html=response.read().decode()
        assert '<dui-updates ref="updates">' in html
        with opener.open(base+'/assets/js/util/updates.js',timeout=10) as response:
            assert b'const DuiUpdates' in response.read()
        print(json.dumps({'authenticatedRoutes':True,'crossOriginRejected':True,'unsupportedInstallSafe':True,'servedComponent':True}))
    finally:
        os.killpg(proc.pid,signal.SIGTERM)
        try:proc.wait(timeout=15)
        except subprocess.TimeoutExpired:os.killpg(proc.pid,signal.SIGKILL);proc.wait()
        log.close()
