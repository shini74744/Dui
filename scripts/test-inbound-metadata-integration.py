"""Exercise metadata updates against an isolated panel/core and a live VLESS stream."""
import http.cookiejar,json,os,shutil,socket,sqlite3,subprocess,tempfile,time,urllib.parse,urllib.request,uuid,threading,hashlib,signal
from pathlib import Path
def port():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
uid=65534 if os.geteuid()==0 else os.getuid()
gid=65534 if os.geteuid()==0 else os.getgid()
def drop_root():
 if os.geteuid()==0:os.setgroups([]);os.setgid(gid);os.setuid(uid)
out=Path(os.environ.get('DUI_PROBE_TEST_OUTPUT','/tmp'))
with tempfile.TemporaryDirectory(prefix='dui-metadata-') as tmp:
 root=Path(tmp);os.chown(root,uid,gid)
 for name in ('db','bin','logs'):
  (root/name).mkdir();os.chown(root/name,uid,gid)
 for source,target in [(os.environ['DUI_TEST_PANEL'],root/'x-ui'),(os.environ['DUI_TEST_CORE'],root/'bin/xray-linux-amd64')]:
  shutil.copy2(source,target);target.chmod(0o755)
 env=os.environ.copy();env.update(XUI_DB_FOLDER=str(root/'db'),XUI_BIN_FOLDER=str(root/'bin'),XUI_LOG_FOLDER=str(root/'logs'),XUI_LOG_LEVEL='error')
 panel_port,api_port,inbound_port=port(),port(),port();password=uuid.uuid4().hex
 subprocess.run([str(root/'x-ui'),'setting','-username','metadata-test','-password',password,'-port',str(panel_port),'-webBasePath','/fixture/','-listenIP','127.0.0.1'],cwd=root,env=env,preexec_fn=drop_root,capture_output=True,check=True,timeout=30)
 db=root/'db/x-ui.db'
 template={'log':{'loglevel':'debug','access':'none','error':str(root/'logs/core.log')},
  'api':{'tag':'api','services':['HandlerService','StatsService']},'stats':{},
  'inbounds':[{'tag':'api','listen':'127.0.0.1','port':api_port,'protocol':'tunnel','settings':{'address':'127.0.0.1'}}],
  'outbounds':[{'tag':'direct','protocol':'freedom','settings':{'finalRules':[{'action':'allow','ip':['127.0.0.1/32']}]}},{'tag':'api','protocol':'blackhole'}],
  'routing':{'rules':[{'type':'field','inboundTag':['api'],'outboundTag':'api'},
    {'type':'field','user':['alpha-user'],'outboundTag':'direct'},
    {'type':'field','user':['regexp:^alpha-'],'outboundTag':'direct'},
    {'type':'field','user':['alpha-user'],'inboundTag':['other-inbound'],'outboundTag':'direct'}]},
  'policy':{'levels':{'0':{'bufferSize':64}},'system':{'statsInboundUplink':True}}}
 with sqlite3.connect(db) as conn:conn.execute('INSERT INTO settings(key,value) VALUES(?,?)',('xrayTemplateConfig',json.dumps(template)))
 log=(root/'panel.log').open('w');proc=subprocess.Popen([str(root/'x-ui')],cwd=root,env=env,preexec_fn=drop_root,start_new_session=True,stdout=log,stderr=log)
 jar=http.cookiejar.CookieJar();opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar));base='http://127.0.0.1:'+str(panel_port)+'/fixture'
 def request(path,data=None):
  payload=None if data is None else urllib.parse.urlencode(data).encode()
  with opener.open(base+path,payload,timeout=30) as r:return json.load(r)
 def core_identity():
  result=[]
  for child in Path('/proc').glob('[0-9]*'):
   try:
    if str(root/'bin/xray-linux-amd64').encode() not in (child/'cmdline').read_bytes():continue
    result.append((int(child.name),(child/'stat').read_text().rsplit(')',1)[1].split()[19]))
   except (FileNotFoundError,ProcessLookupError,PermissionError):pass
  return result
 def listener_inode(p):
  for line in Path('/proc/net/tcp').read_text().splitlines()[1:]:
   cols=line.split()
   if int(cols[1].split(':')[1],16)==p and cols[3]=='0A':return cols[9]
  return None
 echo=socket.socket();echo.bind(('127.0.0.1',0));echo.listen();echo.settimeout(.5);stop=threading.Event()
 def echo_one(c):
  try:
   with c:
    while True:
     data=c.recv(65536)
     if not data:return
     c.sendall(data)
  except OSError:pass
 def echo_loop():
  while not stop.is_set():
   try:c,_=echo.accept();threading.Thread(target=echo_one,args=(c,),daemon=True).start()
   except socket.timeout:continue
   except OSError:return
 threading.Thread(target=echo_loop,daemon=True).start()
 user_id=str(uuid.uuid4());client={'id':user_id,'email':'alpha-user','enable':True,'totalGB':0,'expiryTime':0,'subId':'unchanged-sub'}
 def connect_vless(p):
  c=socket.create_connection(('127.0.0.1',p),timeout=3);c.settimeout(3)
  c.sendall(b'\0'+uuid.UUID(user_id).bytes+b'\0\1'+echo.getsockname()[1].to_bytes(2,'big')+b'\1'+socket.inet_aton('127.0.0.1')+b'hello')
  received=b''
  while len(received)<7:
   chunk=c.recv(7-len(received));assert chunk,"VLESS handshake closed";received+=chunk
  assert received==b'\0\0hello',received
  return c
 def ping(c,text):
  data=text.encode();c.sendall(data);received=b''
  while len(received)<len(data):
   chunk=c.recv(len(data)-len(received));assert chunk,'existing stream closed';received+=chunk
  assert received==data
 try:
  for _ in range(100):
   assert proc.poll() is None
   try:
    with socket.create_connection(('127.0.0.1',panel_port),timeout=.1):break
   except OSError:time.sleep(.1)
  assert request('/login',{'username':'metadata-test','password':password})['success']
  # The HTTP listener can be ready before the core API has finished starting.
  for _ in range(100):
   assert proc.poll() is None
   if listener_inode(api_port) and core_identity():break
   time.sleep(.1)
  else:raise AssertionError('isolated core API did not start')
  row={'remark':'original','enable':'true','listen':'127.0.0.1','port':inbound_port,'protocol':'vless','up':0,'down':0,'total':0,'expiryTime':0,'trafficReset':'never','deviceLimit':0,
    'settings':json.dumps({'clients':[client],'decryption':'none','fallbacks':[]}),
    'streamSettings':json.dumps({'network':'tcp','security':'none'}),'sniffing':'{"enabled":false}'}
  res=request('/dui/api/inbounds/add',row);assert res['success'],res
  saved=request('/dui/api/inbounds/list')['obj'][0];inbound_id=saved['id']
  print('isolated inbound added',flush=True)
  for _ in range(100):
   try:stream=connect_vless(inbound_port);break
   except (ConnectionError,OSError) as err:
    print(type(err).__name__,flush=True);time.sleep(.1)
  else:raise AssertionError('isolated core inbound did not start')
  identity=core_identity();inode=listener_inode(inbound_port);assert identity and inode
  refs=request('/dui/api/inbounds/clientRouteReferences');assert refs['success'],refs
  assert [r['index'] for r in refs['obj'][0]['rules']]==[2,3],refs
  with sqlite3.connect(db) as conn:before=conn.execute('SELECT * FROM inbounds WHERE id=?',(inbound_id,)).fetchone();cols=[r[1] for r in conn.execute('PRAGMA table_info(inbounds)')]
  for i in range(12):
   assert request('/dui/api/inbounds/updateRemark/'+str(inbound_id),{'remark':'renamed-'+str(i),'port':1,'settings':'{}'})['success']
   ping(stream,'ongoing-traffic-'+str(i))
   assert listener_inode(inbound_port)==inode,'listener was recreated'
   assert core_identity()==identity,'core restarted'
  with sqlite3.connect(db) as conn:
   after=conn.execute('SELECT * FROM inbounds WHERE id=?',(inbound_id,)).fetchone()
   for key,a,b in zip(cols,before,after):
    if key not in ('remark','up','down','all_time'):assert a==b,('column changed',key)
   assert after[cols.index('remark')]=='renamed-11'
  assert not request('/dui/api/inbounds/updateRemark/'+str(inbound_id),{})['success']
  assert not request('/dui/api/inbounds/updateRemark/99999',{'remark':'missing'})['success']
  renamed={**client,'email':'alpha-renamed'}
  result=request('/dui/api/inbounds/updateClient/'+user_id,{'id':inbound_id,'settings':json.dumps({'clients':[renamed]})});assert result['success'],result
  ping(stream,'stream-after-email-rename');assert core_identity()==identity
  with connect_vless(inbound_port) as new_stream:ping(new_stream,'new-connection-after-rename')
  refs=request('/dui/api/inbounds/clientRouteReferences')
  assert refs['obj'][0]['email']=='alpha-renamed' and [r['index'] for r in refs['obj'][0]['rules']]==[3],refs
  with sqlite3.connect(db) as conn:assert json.loads(conn.execute("SELECT value FROM settings WHERE key='xrayTemplateConfig'").fetchone()[0])==template
  stream.close()
  # Real node edits must still use the normal save path and update the listener.
  changed=request('/dui/api/inbounds/list')['obj'][0];new_port=port();changed['port']=new_port;changed['enable']='true'
  changed={k:v for k,v in changed.items() if k not in ('clientStats','tag')}
  res=request('/dui/api/inbounds/update/'+str(inbound_id),changed);assert res['success'],res
  with connect_vless(new_port) as c:ping(c,'port-change-applied')
  assert listener_inode(inbound_port) is None
  if os.environ.get('DUI_METADATA_BROWSER'):
   browser_env=os.environ.copy();browser_env.update(DUI_FIXTURE_BASE=base,DUI_FIXTURE_COOKIES=json.dumps([{'name':c.name,'value':c.value,'url':base+'/'} for c in jar]))
   subprocess.run(['node',str(Path(__file__).with_name('test-inbound-metadata-ui.cjs'))],env=browser_env,check=True,timeout=240)
   subprocess.run(['node',str(Path(__file__).with_name('test-inbound-search-ui.cjs'))],env=browser_env,check=True,timeout=120)
  report={'passed':True,'remarkSaves':12,'existingStreamPreserved':True,'listenerUnchanged':True,'coreUnchanged':True,'emailHotUpdateStreamPreserved':True,'configurationEditApplied':True,'routeReferencesPreserved':True}
  (out/'integration.json').write_text(json.dumps(report,indent=2));print(json.dumps(report))
 except BaseException:
  log.flush();shutil.copy2(root/'panel.log',out/'integration-failure.log')
  if (root/'logs/core.log').exists():shutil.copy2(root/'logs/core.log',out/'isolated-core-failure.log')
  raise
 finally:
  stop.set();echo.close()
  try:os.killpg(proc.pid,signal.SIGTERM)
  except ProcessLookupError:pass
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:os.killpg(proc.pid,signal.SIGKILL);proc.wait()
  log.close()
