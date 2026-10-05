"""Old-panel upgrade regression using isolated databases, loopback ports and an unprivileged UID.
Required: DUI_TEST_PANEL, DUI_TEST_CORE, DUI_TEST_OLD_CORE, DUI_TEST_OLD_PANELS (colon-separated).
Never addresses an installed systemd service or database.
"""
import http.cookiejar, json, os, shutil, signal, socket, sqlite3, subprocess, tempfile, time, urllib.parse, urllib.request, uuid
from pathlib import Path

def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1',0)); return s.getsockname()[1]
def drop():
    if os.geteuid()==0:
        os.setgroups([]); os.setgid(65534); os.setuid(65534)
def stop(proc):
    if proc and proc.poll() is None:
        os.killpg(proc.pid,signal.SIGTERM)
        try: proc.wait(timeout=15)
        except subprocess.TimeoutExpired: os.killpg(proc.pid,signal.SIGKILL); proc.wait(timeout=5)
def version(p):
    return subprocess.check_output([p,'-v'],text=True).strip()

reports=[]
for old in os.environ['DUI_TEST_OLD_PANELS'].split(':'):
    for broken, base_path in [(False,'/legacy-kept/'),(True,'/')]:
        with tempfile.TemporaryDirectory(prefix='dui-upgrade-compat-') as temp:
            root=Path(temp); root.chmod(0o755)
            for n in ['db','bin','logs']:
                (root/n).mkdir()
                if os.geteuid()==0: os.chown(root/n,65534,65534)
            for src,dst in [(old,root/'panel'),(os.environ['DUI_TEST_OLD_CORE'],root/'bin/xray-linux-amd64')]:
                shutil.copy2(src,dst);dst.chmod(0o755)
            env=os.environ.copy()
            env.update(XUI_DB_FOLDER=str(root/'db'),XUI_BIN_FOLDER=str(root/'bin'),XUI_LOG_FOLDER=str(root/'logs'),XUI_LOG_LEVEL='error')
            pp,ap,ip=port(),port(),port();password=uuid.uuid4().hex
            r=subprocess.run([str(root/'panel'),'setting','-username','compat-user','-password',password,'-port',str(pp),'-webBasePath',base_path,'-listenIP','127.0.0.1'],cwd=root,env=env,preexec_fn=drop,capture_output=True,timeout=30)
            assert r.returncode==0,'old database initialization failed'
            template={
                'log':{'loglevel':'none','access':'none','error':'none'},
                'api':{'tag':'api','services':['HandlerService','StatsService']},'stats':{},
                'inbounds':[{'tag':'api','listen':'127.0.0.1','port':ap,'protocol':'tunnel','settings':{'address':'127.0.0.1'}}],
                'outbounds':[{'tag':'direct','protocol':'freedom'},{'tag':'api','protocol':'blackhole'}],
                'routing':{'rules':[{'type':'field','inboundTag':['api'],'outboundTag':'api'},{'type':'field','domain':['example.invalid'],'outboundTag':'direct'}]},
                'policy':{'levels':{'0':{'handshake':4,'connIdle':300,'uplinkOnly':0,'downlinkOnly':0}},'system':{'duiCloseWaitTimeout':60}}}
            if broken:
                template['outbounds'].append({'tag':'legacy-plain-vless','protocol':'vless','settings':{'vnext':[{'address':'8.8.8.8','port':443,'users':[{'id':str(uuid.uuid4()),'encryption':'none'}]}]},'streamSettings':{'network':'tcp','security':'none'}})
            db=root/'db/x-ui.db'
            with sqlite3.connect(db) as c:
                c.execute("INSERT INTO settings(key,value) VALUES('xrayTemplateConfig',?)",(json.dumps(template),))
                columns={r[1] for r in c.execute('PRAGMA table_info(inbounds)')}
                inbound={'id':1,'user_id':1,'enable':1,'remark':'compat-inbound','listen':'127.0.0.1','port':ip,'protocol':'vless','settings':json.dumps({'clients':[{'id':str(uuid.uuid4()),'email':'compat@example.invalid','enable':True}],'decryption':'none'}),'stream_settings':json.dumps({'network':'tcp','security':'none'}),'sniffing':'{}','up':0,'down':0,'total':0,'expiry_time':0,'tag':'inbound-1'}
                inbound={k:v for k,v in inbound.items() if k in columns}
                c.execute('INSERT INTO inbounds('+','.join(inbound)+') VALUES('+','.join('?' for _ in inbound)+')',list(inbound.values()))
            def snapshot():
                with sqlite3.connect(db) as c:
                    settings=c.execute("SELECT key,value FROM settings WHERE key IN ('webPort','webBasePath','webListen','xrayTemplateConfig') ORDER BY key").fetchall()
                    users=c.execute('SELECT id,username,password FROM users ORDER BY id').fetchall()
                    inbound=c.execute('SELECT id,enable,remark,listen,port,protocol,settings,stream_settings,sniffing FROM inbounds ORDER BY id').fetchall()
                    return settings,users,inbound
            initial=snapshot()
            log=(root/'test.log').open('w');proc=None
            base='http://127.0.0.1:'+str(pp)
            opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
            def request(path,data=None,as_json=False):
                headers={'Origin':base}
                if data is not None:
                    if as_json: data=json.dumps(data).encode();headers['Content-Type']='application/json'
                    else: data=urllib.parse.urlencode(data).encode()
                with opener.open(urllib.request.Request(base+base_path+path.lstrip('/'),data,headers),timeout=15) as r:return json.load(r)
            def start():
                proc=subprocess.Popen([str(root/'panel')],cwd=root,env=env,preexec_fn=drop,start_new_session=True,stdout=log,stderr=log)
                for _ in range(150):
                    assert proc.poll() is None,'isolated panel exited'
                    try:
                        with socket.create_connection(('127.0.0.1',pp),timeout=.1):break
                    except OSError:time.sleep(.1)
                assert request('login',{'username':'compat-user','password':password})['success'],'account did not survive upgrade'
                return proc
            try:
                proc=start()
                time.sleep(.6)
                assert (root/'bin/config.json').exists()!=broken,'old-core failure scenario not reproduced'
                stop(proc)
                shutil.copy2(os.environ['DUI_TEST_PANEL'],root/'panel')
                proc=start()
                status=request('dui/api/server/updates')['obj']
                core=status['items']['core']
                assert core['current']=='vx-26.6' and core['currentKnown'],'installed stopped core not recognized'
                assert not core['upToDate'],'unchecked core claimed up to date'
                if broken:
                    assert not (root/'bin/config.json').exists(),'status created live configuration'
                    answer=request('dui/api/server/updates/start',{'kind':'core','version':'vx-26.7'},True)
                    assert not answer['success'] and 'unsupported_installation' in answer.get('msg',''),'missing-config recovery failed before unprivileged/systemd guard'
                    assert not (root/'bin/config.json').exists(),'preflight wrote live config'
                assert snapshot()==initial,'settings, account or inbound changed after panel upgrade'
                stop(proc)
                shutil.copy2(os.environ['DUI_TEST_CORE'],root/'bin/xray-linux-amd64')
                proc=start()
                for _ in range(60):
                    if (root/'bin/config.json').exists():break
                    time.sleep(.1)
                status=request('dui/api/server/updates')['obj']
                assert status['items']['core']['current']=='vx-26.7'
                runtime=None
                for _ in range(50):
                    state=request('dui/api/server/status').get('obj')
                    if state and state.get('xray',{}).get('state')=='running':
                        runtime=state['xray'];break
                    time.sleep(.2)
                assert runtime and runtime['state']=='running','new core did not become running'
                assert snapshot()==initial,'settings, account or inbound changed after core upgrade'
                reports.append({'fromPanel':version(old),'coreWasStopped':broken,'basePath':base_path,'accountPortPathInboundsRoutesPolicyPreserved':True,'installedCore':'vx-26.7','coreRunning':True,'noLiveConfigWrittenDuringPreflight':broken})
                print(json.dumps(reports[-1]),flush=True)
            finally:
                stop(proc);log.close()
print(json.dumps({'oldVersionUpgradeCases':len(reports),'passed':True}),flush=True)
