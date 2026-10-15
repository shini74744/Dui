"""Isolated route fallback and priority API/browser regression. Never uses an installed database/config.

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
                             '-port', str(panel_port), '-webBasePath', '/fixture/', '-listenIP', '127.0.0.1'],
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
    base = 'http://127.0.0.1:'+str(panel_port)+'/fixture'

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
        # Parked probe parameters must survive real API/database reload and never reach runtime.
        parked=saved()
        parked['duiDisabledProbes']={'outbound':{'subjectSelector':['direct'],'pingConfig':{'interval':'17m','future':'keep'},'custom':{'keep':True}}}
        assert request('/dui/xray/update',{'xraySetting':json.dumps(parked)})['success']
        assert saved()['duiDisabledProbes']==parked['duiDisabledProbes']
        response=request('/dui/xray/',{})
        assert json.loads(response['obj'])['xraySetting']['duiDisabledProbes']==parked['duiDisabledProbes']
        assert request('/dui/api/server/restartXrayService',{})['success']
        runtime=json.loads((root/'bin/config.json').read_text())
        assert 'duiDisabledProbes' not in runtime
        assert 'outbound' not in runtime.get('strategyObservatory',{})
        # Native fallback and priority configuration must survive save/reload.
        current=saved()
        current['outbounds'] += [{'tag':tag,'protocol':'freedom'} for tag in ['a','b','c','d']]
        probe=lambda tags:{'subjectSelector':tags,'pingConfig':{'destination':'http://127.0.0.1:1/probe','interval':'1m','timeout':'1s','sampling':1,'connectivity':''}}
        current['strategyObservatory']={
            'outbound':probe(['a','b']),
            'roundRobin':probe(['a','b','direct']),
            'leastPing':{'subjectSelector':['c','d'],'probeURL':'http://127.0.0.1:1/probe','probeInterval':'1m','enableConcurrency':True}}
        current['routing']['balancers']=[{'tag':'tiers','strategy':{'type':'priority','settings':{'levels':[
            {'selector':['a','b'],'strategy':{'type':'roundRobin'}},
            {'selector':['c','d'],'strategy':{'type':'leastPing'}},
            {'selector':['direct'],'strategy':{'type':'roundRobin'}}]}}}]
        current['routing']['rules'] += [
            {'type':'field','inboundTag':['fixture-primary'],'outboundTag':'a','fallbackTag':'b'},
            {'type':'field','inboundTag':['fixture-priority'],'balancerTag':'tiers'}]
        r=request('/dui/xray/update',{'xraySetting':json.dumps(current)})
        assert r['success'],r.get('msg')
        assert saved()['routing']==current['routing']
        before=saved()
        for change in ('missing','same','no-probe','no-level','nested','unknown-outbound'):
            bad=json.loads(json.dumps(current))
            if change=='missing':bad['routing']['rules'][-2]['fallbackTag']='missing'
            if change=='same':bad['routing']['rules'][-2]['fallbackTag']='a'
            if change=='no-probe':del bad['strategyObservatory']['outbound']
            if change=='no-level':bad['routing']['balancers'][0]['strategy']['settings']['levels']=[]
            if change=='nested':bad['routing']['balancers'][0]['strategy']['settings']['levels'][0]['strategy']['type']='priority'
            if change=='unknown-outbound':bad['routing']['balancers'][0]['strategy']['settings']['levels'][0]['selector']=['missing']
            r=request('/dui/xray/update',{'xraySetting':json.dumps(bad)})
            assert not r['success'],change
            assert saved()==before,change+' changed saved configuration'
        # A core downgrade is refused before disturbing the running service.
        r=request('/dui/installXray/vx-26.9',{})
        assert not r['success'] and 'vx-26.10' in r['msg']
        if os.environ.get('DUI_HEALTH_BROWSER'):
            cookies=[]
            for handler in opener.handlers:
                if isinstance(handler,urllib.request.HTTPCookieProcessor):
                    cookies=[{'name':c.name,'value':c.value,'url':base+'/'} for c in handler.cookiejar]
            browser_env=os.environ.copy()
            browser_env.update(DUI_FIXTURE_BASE=base,DUI_FIXTURE_COOKIES=json.dumps(cookies))
            if not os.environ.get('DUI_SKIP_ROUTE_UI'):
                subprocess.run(['node',str(Path(__file__).with_name('test-route-health-ui.cjs'))],env=browser_env,check=True,timeout=240)
            if os.environ.get('DUI_INBOUND_SEARCH_BROWSER'):
                subprocess.run(['node',str(Path(__file__).with_name('test-inbound-search-ui.cjs'))],env=browser_env,check=True,timeout=120)
            if os.environ.get('DUI_SPEED_BROWSER'):
                subprocess.run(['node',str(Path(__file__).with_name('test-speedtest-menu-ui.cjs'))],env=browser_env,check=True,timeout=120)
            if os.environ.get('DUI_PROBE_CONTROLS_BROWSER'):
                # Synthetic history is inserted only in this disposable test database.
                with sqlite3.connect(db) as conn:
                    for i,(scope,tag) in enumerate([('outbound','a'),('roundRobin','a'),('legacyBurst','a'),('outbound','b')]):
                        conn.execute('INSERT INTO probe_histories(session,sequence,time,strategy,outbound,status,delay) VALUES(?,?,?,?,?,?,?)',('ui-fixture',i+1,int(time.time()*1000),scope,tag,'success',12.5))
                subprocess.run(['node',str(Path(__file__).with_name('test-probe-controls-ui.cjs'))],env=browser_env,check=True,timeout=240)

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
