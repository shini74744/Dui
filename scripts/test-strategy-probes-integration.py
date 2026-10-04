"""Isolated strategy probe API regression. Never uses an installed database/config.

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
                with socket.create_connection(('127.0.0.1', panel_port), timeout=.1): break
            except OSError: time.sleep(.1)
        assert request('/login', {'username': 'cw-test', 'password': password})['success']
        current=saved()
        current['observatory']={'subjectSelector':[], 'probeInterval':'10m'}
        current['burstObservatory']={'subjectSelector':[], 'pingConfig':{'interval':'30m','sampling':2,'timeout':'10s','connectivity':''}}
        current['routing']['balancers']=[]
        current['strategyObservatory']={}
        for i,name in enumerate(('leastPing','leastLoad','random','roundRobin')):
            current['routing']['balancers'].append({'tag':name,'selector':['direct'],'strategy':{'type':name}})
            # No external requests: the selected observer targets are empty in this API persistence test.
            if name=='leastPing':
                current['strategyObservatory'][name]={'subjectSelector':[],'probeURL':'http://127.0.0.1:1/ping','probeInterval':'1m','enableConcurrency':True}
            else:
                current['strategyObservatory'][name]={'subjectSelector':[],'pingConfig':{'destination':'http://127.0.0.1:1/'+name,'interval':str(i+1)+'m','timeout':'2s','sampling':2,'connectivity':''}}
        r=request('/dui/xray/update',{'xraySetting':json.dumps(current)})
        assert r['success'],r.get('msg')
        assert saved()['strategyObservatory']==current['strategyObservatory']
        r=request('/dui/xray/',{})
        obj=json.loads(r['obj'])
        assert obj['xraySetting']['strategyObservatory']==current['strategyObservatory']
        r=request('/dui/api/server/restartXrayService',{})
        assert r['success'],r.get('msg')
        for _ in range(60):
            runtime=json.loads((root/'bin/config.json').read_text())
            if runtime.get('strategyObservatory')==current['strategyObservatory']:break
            time.sleep(.1)
        assert runtime['strategyObservatory']==current['strategyObservatory']
        before=saved()
        bad=json.loads(json.dumps(current))
        bad['strategyObservatory']['random']['pingConfig']['sampling']=0
        assert not request('/dui/xray/update',{'xraySetting':json.dumps(bad)})['success']
        assert saved()==before
        original=binary.with_suffix('.new-core')
        binary.rename(original)
        binary.write_text('#!/bin/sh\nif [ "$1" = "-version" ]; then echo "Xray vx-26.4 (Xray)"; exit 0; fi\nexit 19\n')
        binary.chmod(0o755)
        try:
            r=request('/dui/xray/update',{'xraySetting':json.dumps(current)})
            assert not r['success'] and 'vx-26.5' in r['msg']
            assert saved()==before
        finally:
            binary.unlink()
            original.rename(binary)
        r=request('/dui/installXray/vx-26.4',{})
        assert not r['success'] and 'vx-26.5' in r['msg']
        with socket.create_connection(('127.0.0.1',api_port),timeout=2): pass
        with opener.open(base+'/dui/xray',timeout=20) as r: html=r.read().decode()
        assert 'dui-strategy-probes-template' in html and 'strategy-probes-i18n.js' in html
        assert 'ZgotmplZ' not in html
        if os.environ.get('DUI_CAPTURE_HTML'):Path(os.environ['DUI_CAPTURE_HTML']).write_text(html)
        for asset in ('strategy-probes.js','strategy-probes-i18n.js'):
            with opener.open(base+'/assets/js/util/'+asset,timeout=10) as r: content=r.read()
            expected=Path(__file__).resolve().parent.parent/'web/assets/js/util'/asset
            assert content==expected.read_bytes()
        legacy=json.loads(json.dumps(current));del legacy['strategyObservatory']
        assert request('/dui/xray/update',{'xraySetting':json.dumps(legacy)})['success']
        assert 'strategyObservatory' not in saved()
        print(json.dumps({'save_reload':True,'runtime_preserved':True,'invalid_preserves_database':True,
                          'old_core_rejected':True,'downgrade_rejected_before_stop':True,
                          'templates_and_assets_verified':True,'legacy_restore':True}))
    finally:
        if proc.poll() is None:
            os.killpg(proc.pid, signal.SIGTERM)
            try: proc.wait(timeout=8)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid,signal.SIGKILL)
                proc.wait()
        log.close()
