"""Isolated real probe history and persistence regression. Never uses an installed database/config.

Set DUI_TEST_PANEL and DUI_TEST_CORE to the binaries being tested.
The temporary panel binds loopback and runs without root privileges.
"""
import http.cookiejar
import http.server
import threading
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


class ProbeHandler(http.server.BaseHTTPRequestHandler):
    def do_HEAD(self):
        if self.path in ('/failed','/network','/connectivity'):
            self.connection.shutdown(socket.SHUT_RDWR)
            self.connection.close()
            return
        self.send_response(204)
        self.end_headers()
    do_GET=do_HEAD
    def log_message(self,*args): pass

probe_server=http.server.ThreadingHTTPServer(('127.0.0.1',0),ProbeHandler)
threading.Thread(target=probe_server.serve_forever,daemon=True).start()
probe_url='http://127.0.0.1:'+str(probe_server.server_port)

with tempfile.TemporaryDirectory(prefix='dui-history-api-') as tmp:
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
    template['routing']['balancers']=[]
    template['strategyObservatory']={}
    for name in ('leastPing','leastLoad','random','roundRobin'):
        template['routing']['balancers'].append({'tag':name,'selector':['direct'],'strategy':{'type':name}})
        if name=='leastPing':
            template['strategyObservatory'][name]={'subjectSelector':['direct'],'probeURL':probe_url+'/success','probeInterval':'1s','enableConcurrency':True}
        else:
            path={'leastLoad':'/network','random':'/failed','roundRobin':'/success'}[name]
            template['strategyObservatory'][name]={'subjectSelector':['direct'],'pingConfig':{'destination':probe_url+path,'interval':'10s','timeout':'1s','sampling':1,'connectivity':probe_url+'/connectivity' if name=='leastLoad' else ''}}
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

    def history(strategy='leastPing',page=1,tag=''):
        query=urllib.parse.urlencode({'strategy':strategy,'page':page,'outbound':tag})
        with opener.open(base+'/dui/xray/probeHistory?'+query,timeout=15) as response:
            return json.load(response)

    def login():
        for _ in range(100):
            assert proc.poll() is None,'isolated panel exited'
            try:
                with socket.create_connection(('127.0.0.1',panel_port),timeout=.1):break
            except OSError:time.sleep(.1)
        assert request('/login',{'username':'cw-test','password':password})['success']

    try:
        login()
        expected={'leastPing':'success','leastLoad':'network_unavailable','random':'failed','roundRobin':'success'}
        pages={}
        deadline=time.monotonic()+25
        while time.monotonic()<deadline:
            pages={strategy:history(strategy)['obj'] for strategy in expected}
            if all(page['rows'] for page in pages.values()):break
            time.sleep(.5)
        for strategy,state in expected.items():
            assert pages[strategy]['rows'],strategy+' has no completed probes'
            assert all(row['strategy']==strategy and row['status']==state for row in pages[strategy]['rows']),strategy+' leaked or mislabeled results'
            assert pages[strategy]['retentionDays']==7 and pages[strategy]['limit']==100000
            assert pages[strategy]['state']=='ok'
        first_ids={row['id'] for row in pages['leastPing']['rows']}
        with sqlite3.connect(db) as conn:
            before=conn.execute("select count(*) from probe_histories").fetchone()[0]
            distinct=conn.execute("select count(*) from (select distinct session,sequence from probe_histories)").fetchone()[0]
            assert before==distinct,'collector duplicated events'
        assert not history('invalid')['success']
        assert not history(page=-1)['success']
        assert history(tag='missing')['obj']['total']==0
        # Authenticated API only: an anonymous request must not expose rows.
        with urllib.request.urlopen(base+'/dui/xray/probeHistory?strategy=random',timeout=10) as response:
            anonymous=response.read().decode()
        assert '"rows"' not in anonymous and '"sequence"' not in anonymous
        # Restart the entire isolated panel, not just the core, to prove durable history.
        os.killpg(proc.pid,signal.SIGTERM)
        proc.wait(timeout=15)
        proc=subprocess.Popen([str(root/'x-ui')],cwd=root,env=env,preexec_fn=drop_root,start_new_session=True,stdout=log,stderr=log)
        login()
        rows=history()['obj']['rows']
        assert first_ids.issubset({row['id'] for row in rows}),'history vanished on panel restart'
        deadline=time.monotonic()+15
        while time.monotonic()<deadline:
            with sqlite3.connect(db) as conn:
                sessions=conn.execute("select count(distinct session) from probe_histories").fetchone()[0]
            if sessions>=2:break
            time.sleep(.5)
        assert sessions>=2,'new core session was not collected'
        assert saved()==template,'history collection changed core settings'
        print(json.dumps({'four_real_probe_streams':True,'failure_and_network_down_distinguished':True,'deduplicated':True,'panel_restart_retains_history':True,'new_core_session_collected':True,'auth_required':True,'filters_validated':True,'configuration_unchanged':True}))
    finally:
        if proc.poll() is None:
            os.killpg(proc.pid,signal.SIGTERM)
            try:proc.wait(timeout=12)
            except subprocess.TimeoutExpired:os.killpg(proc.pid,signal.SIGKILL);proc.wait()
        log.close()
        probe_server.shutdown()
