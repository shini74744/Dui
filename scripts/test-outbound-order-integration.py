"""Isolated outbound order save/reload regression. Never uses an installed database/config.

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


with tempfile.TemporaryDirectory(prefix='dui-outbound-order-api-') as tmp:
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
                with socket.create_connection(('127.0.0.1', panel_port), timeout=.1): break
            except OSError: time.sleep(.1)
        assert request('/login', {'username': 'cw-test', 'password': password})['success']
        current=saved()
        current['outbounds'].extend([
            {'tag':'second','protocol':'freedom','settings':{'domainStrategy':'AsIs'}},
            {'tag':'third','protocol':'freedom'}])
        current['routing']['rules'].append({'type':'field','domain':['domain:example.test'],'outboundTag':'second'})
        current['routing']['balancers']=[{'tag':'group','selector':['second','third'],'strategy':{'type':'random'}}]
        before=json.loads(json.dumps(current))
        current['outbounds']=[current['outbounds'][2],current['outbounds'][0],current['outbounds'][3],current['outbounds'][1]]
        r=request('/dui/xray/update',{'xraySetting':json.dumps(current)})
        assert r['success'],r.get('msg')
        assert saved()['outbounds']==current['outbounds']
        assert saved()['routing']==before['routing']
        r=request('/dui/xray/',{})
        loaded=json.loads(r['obj'])['xraySetting']
        assert loaded['outbounds']==current['outbounds']
        assert request('/dui/api/server/restartXrayService',{})['success']
        for _ in range(80):
            runtime=json.loads((root/'bin/config.json').read_text())
            if [o.get('tag') for o in runtime['outbounds']][:4]==['second','direct','third','api']: break
            time.sleep(.1)
        assert [o.get('tag') for o in runtime['outbounds']][:4]==['second','direct','third','api']
        assert runtime['routing']['rules'][-1]['outboundTag']=='second'
        # Real rendered template and served assets, without reading any production data.
        with opener.open(base+'/dui/xray',timeout=20) as r: html=r.read().decode()
        assert 'dui-outbound-sort-handle' in html and 'outbound-order.js' in html
        assert 'ZgotmplZ' not in html
        if os.environ.get('DUI_CAPTURE_HTML'): Path(os.environ['DUI_CAPTURE_HTML']).write_text(html)
        with opener.open(base+'/assets/js/util/outbound-order.js',timeout=10) as r: content=r.read()
        assert content==(Path(__file__).resolve().parent.parent/'web/assets/js/util/outbound-order.js').read_bytes()
        print(json.dumps({'save_reload_order':True,'runtime_order':True,'references_unchanged':True,
                          'rendered_template_and_asset':True}))

    finally:
        if proc.poll() is None:
            os.killpg(proc.pid, signal.SIGTERM)
            try: proc.wait(timeout=8)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid,signal.SIGKILL)
                proc.wait()
        log.close()
