import ssl,threading,http.server
import os,json,socket,sqlite3,subprocess,tempfile,time,uuid,signal,shutil,urllib.request,urllib.parse,http.cookiejar
from pathlib import Path
os.umask(0o077)

def port():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
def unprivileged():
 os.setgroups([]);os.setgid(65534);os.setuid(65534)
with tempfile.TemporaryDirectory(prefix='dui-panel-compat-test-') as tmp:
 root=Path(tmp);os.chown(root,65534,65534)
 for name in ('db','bin','logs'):
  d=root/name;d.mkdir();os.chown(d,65534,65534)
 for src,dst in [(Path(os.environ['DUI_TEST_PANEL']),root/'x-ui'),(Path(os.environ['DUI_TEST_CORE']),root/'bin/xray-linux-amd64')]:
  shutil.copy2(src,dst);dst.chmod(0o755)
 if os.environ.get('DUI_TEST_LEGACY_CORE'):
  shutil.copy2(os.environ['DUI_TEST_LEGACY_CORE'],root/'legacy-xray');(root/'legacy-xray').chmod(0o755)
 env=os.environ.copy();env.update(XUI_DB_FOLDER=str(root/'db'),XUI_BIN_FOLDER=str(root/'bin'),XUI_LOG_FOLDER=str(root/'logs'),XUI_LOG_LEVEL='error')
 class Echo(http.server.BaseHTTPRequestHandler):
  def do_GET(self):self.send_response(204);self.end_headers()
  def log_message(self,*args):pass
 httpd=http.server.ThreadingHTTPServer(('127.0.0.1',0),Echo)
 threading.Thread(target=httpd.serve_forever,daemon=True).start()
 camo=http.server.ThreadingHTTPServer(('127.0.0.1',0),Echo)
 subprocess.run(['openssl','req','-x509','-newkey','ec','-pkeyopt','ec_paramgen_curve:prime256v1','-nodes','-days','1','-subj','/CN=example.com','-keyout',str(root/'tls.key'),'-out',str(root/'tls.crt')],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 tls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);tls.minimum_version=ssl.TLSVersion.TLSv1_3;tls.set_ecdh_curve('X25519');tls.set_alpn_protocols(['h2','http/1.1']);tls.load_cert_chain(root/'tls.crt',root/'tls.key')
 camo.socket=tls.wrap_socket(camo.socket,server_side=True)
 threading.Thread(target=camo.serve_forever,daemon=True).start()
 pp=port();password=uuid.uuid4().hex;panel=None
 def cli(args):
  p=subprocess.run([str(root/'x-ui'),*args],cwd=root,env=env,preexec_fn=unprivileged,capture_output=True,timeout=30)
  assert p.returncode==0,'isolated panel setting command failed'
 cli(['setting','-username','dui-smoke','-password',password,'-port',str(pp),'-webBasePath','/','-listenIP','127.0.0.1'])
 db=root/'db/x-ui.db'
 template={'log':{'loglevel':'none','access':'none','error':'none'},'api':{'tag':'api','services':['HandlerService','StatsService']},'stats':{},'inbounds':[{'tag':'api','listen':'127.0.0.1','port':port(),'protocol':'tunnel','settings':{'address':'127.0.0.1'}}],'outbounds':[{'tag':'direct','protocol':'freedom','settings':{'finalRules':[{'action':'allow','ip':['127.0.0.0/8']}]}}],'routing':{'rules':[{'type':'field','inboundTag':['api'],'outboundTag':'api'}]},'policy':{'levels':{'0':{'statsUserUplink':True,'statsUserDownlink':True}},'system':{'statsInboundUplink':True,'statsInboundDownlink':True}}}
 with sqlite3.connect(db) as conn:
  conn.execute('INSERT INTO settings(key,value) VALUES(?,?)',('xrayTemplateConfig',json.dumps(template)))
 def start_panel():
  p=subprocess.Popen([str(root/'x-ui')],cwd=root,env=env,preexec_fn=unprivileged,start_new_session=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
  for _ in range(100):
   if p.poll() is not None:raise AssertionError('isolated panel exited')
   try:
    with socket.create_connection(('127.0.0.1',pp),timeout=.1):return p
   except OSError:time.sleep(.1)
  raise AssertionError('isolated panel did not start')
 def stop_panel(p):
  if p is None:return
  try:os.killpg(p.pid,signal.SIGTERM)
  except ProcessLookupError:pass
  try:p.wait(timeout=8)
  except subprocess.TimeoutExpired:
   os.killpg(p.pid,signal.SIGKILL);p.wait()
 def login():
  global opener
  opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
  assert request('/login',{'username':'dui-smoke','password':password})['success'],'test login failed'
 def request(path,data=None):
  body=None if data is None else urllib.parse.urlencode(data).encode()
  with opener.open('http://127.0.0.1:'+str(pp)+path,body,timeout=25) as r:return json.load(r)
 def api(path,data=None):
  r=request('/dui/api/'+path,data);assert r.get('success'), 'isolated API failed: '+path
  return r.get('obj')
 nodes=[]
 def probe(node, expect=True, legacy=False):
  cp=port();stream=node['stream'];cfg={'log':{'loglevel':'none','access':'none'},'inbounds':[{'listen':'127.0.0.1','port':cp,'protocol':'socks','settings':{}}],'outbounds':[{'protocol':'vless','settings':{'vnext':[{'address':'127.0.0.1','port':node['port'],'users':[{'id':node['user'],'encryption':node['encryption']}]}]},'streamSettings':stream}]}
  path=root/'probe.json';path.write_text(json.dumps(cfg));os.chown(path,65534,65534)
  p=subprocess.Popen([str(root/'legacy-xray') if legacy else str(root/'bin/xray-linux-amd64'),'run','-config',str(path)],cwd=root,env=env,preexec_fn=unprivileged,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
  try:
   for _ in range(60):
    try:
     with socket.create_connection(('127.0.0.1',cp),timeout=.1):break
    except OSError:time.sleep(.1)
   r=subprocess.run(['curl','--noproxy','','-sS','--max-time',('20' if expect else '3'),'--retry',('2' if expect else '0'),'--retry-all-errors','--socks5-hostname','127.0.0.1:'+str(cp),'-o','/dev/null','-w','%{http_code}','http://127.0.0.1:'+str(httpd.server_port)+'/generate_204'],capture_output=True,text=True,timeout=70)
   assert (r.returncode==0 and r.stdout.strip().endswith('204'))==expect,'saved node connectivity did not match mode'
  finally:
   p.terminate()
   try:p.wait(timeout=5)
   except subprocess.TimeoutExpired:p.kill();p.wait()
 try:
  panel=start_panel();login()
  auths=api('server/getNewVlessEnc')['auths'];assert len(auths)==2
  for auth in auths:
   for network in ('tcp','xhttp'):
    sp=port();user=str(uuid.uuid4());email=uuid.uuid4().hex
    stream={'network':network,'security':'none'}
    if network=='xhttp':stream['xhttpSettings']={'path':'/dui-test','mode':'auto'}
    settings={'clients':[{'id':user,'email':email,'enable':True,'speedLimitMbps':3}],'decryption':auth['decryption'],'encryption':auth['encryption'],'selectedAuth':auth['label']}
    form={'listen':'127.0.0.1','port':sp,'protocol':'vless','enable':'true','remark':'DUI isolated smoke','settings':json.dumps(settings),'streamSettings':json.dumps(stream),'sniffing':'{}'}
    created=api('inbounds/add',form);saved=api('inbounds/get/'+str(created['id']))
    stored=json.loads(saved['settings']);assert stored['decryption']==settings['decryption'] and stored['encryption']==settings['encryption']
    node={'id':created['id'],'email':email,'tag':saved['tag'],'port':sp,'user':user,'encryption':auth['encryption'],'stream':stream};nodes.append(node);probe(node)
  print(json.dumps({'isolated_panel_create_save_connect':len(nodes),'both_enc_auths':True,'transports':['tcp','xhttp']}),flush=True)
  pair=api('server/getNewX25519Cert');private=pair['privateKey'];public=pair['publicKey'];assert private and public
  source={'target':'127.0.0.1:'+str(camo.server_port),'serverNames':['example.com']}
  rs={'target':source.get('target',source.get('dest')),'serverNames':source['serverNames'][:1],'privateKey':private,'shortIds':['12345678']}
  stream={'network':'tcp','security':'reality','realitySettings':rs}
  rid_user=str(uuid.uuid4());rid_port=port()
  form={'listen':'127.0.0.1','port':rid_port,'protocol':'vless','enable':'true','remark':'DUI isolated REALITY','settings':json.dumps({'clients':[{'id':rid_user,'email':'dui-reality-test','enable':True}],'decryption':'none'}),'streamSettings':json.dumps(stream),'sniffing':'{}'}
  created=api('inbounds/add',form);rid=created['id']
  reality_node={'port':rid_port,'user':rid_user,'encryption':'none','stream':{'network':'tcp','security':'reality','realitySettings':{'serverName':rs['serverNames'][0],'publicKey':public,'shortId':'12345678','fingerprint':'hellochrome_120'}}}
  if os.environ.get('DUI_TEST_LEGACY_CORE'):probe(reality_node,legacy=True)
  probe(reality_node) # Absent server flag defaults to accepting the old fingerprint.
  for strict in (True,False,True,False):
   rs['duiRequireHybridKeyShare']=strict;form['streamSettings']=json.dumps(stream)
   api('inbounds/update/'+str(rid),form)
   saved=json.loads(api('inbounds/get/'+str(rid))['streamSettings'])['realitySettings']
   assert saved['duiRequireHybridKeyShare']==strict and saved['privateKey']==private and saved['shortIds']==rs['shortIds']
   for fp in ('hellochrome_120','chrome'):
    reality_node['stream']['realitySettings']['fingerprint']=fp;probe(reality_node,expect=not(strict and fp=='hellochrome_120'))
  generated=json.loads((root/'bin/config.json').read_text());ap=next(i['port'] for i in generated['inbounds'] if i['tag']=='api')
  def core_users(node):
   raw=subprocess.check_output([str(root/'bin/xray-linux-amd64'),'api','inbounduser','--server=127.0.0.1:'+str(ap),'-tag='+node['tag'],'-email='+node['email']],text=True,stderr=subprocess.DEVNULL)
   return json.loads(raw)['users']
  def core_pids():
   result=[]
   for entry in Path('/proc').iterdir():
    if entry.name.isdigit():
     try:
      if (entry/'exe').resolve(strict=True)==root/'bin/xray-linux-amd64':result.append(entry.name)
     except OSError:pass
   return sorted(result)
  before_pid=core_pids();assert len(before_pid)==1
  for node in nodes:
   assert core_users(node)[0]['level']==(1<<31)|375000
   saved=api('inbounds/get/'+str(node['id']));settings=json.loads(saved['settings'])
   for mbps in (5,3):
    settings['clients'][0]['speedLimitMbps']=mbps
    edit={'listen':'127.0.0.1','port':node['port'],'protocol':'vless','enable':'true','remark':'DUI hot edit','settings':json.dumps(settings),'streamSettings':json.dumps(node['stream']),'sniffing':'{}'}
    api('inbounds/update/'+str(node['id']),edit)
    assert core_users(node)[0]['level']==(1<<31)|int(mbps*1000000/8),'rate lost on inbound hot edit'
   result=subprocess.run([str(root/'bin/xray-linux-amd64'),'api','rmu','--server=127.0.0.1:'+str(ap),'-tag='+node['tag'],node['email']],capture_output=True,text=True,timeout=15)
   assert result.returncode==0 and 'Removed 1 user' in result.stdout
   with sqlite3.connect(db) as conn:conn.execute('UPDATE client_traffics SET enable=0 WHERE email=?',(node['email'],))
   api('inbounds/'+str(node['id'])+'/resetClientTraffic/'+node['email'],{})
   assert core_users(node)[0]['level']==(1<<31)|375000,'rate lost on traffic reset'
   probe(node)
  assert core_pids()==before_pid,'restore unexpectedly restarted core'
  print(json.dumps({'traffic_reset_restore_rate_preserved':4,'hot_inbound_rate_edits':8,'core_pid_unchanged':True}),flush=True)
  stop_panel(panel);panel=None;panel=start_panel();login()
  for node in nodes:probe(node)
  saved=json.loads(api('inbounds/get/'+str(rid))['streamSettings'])['realitySettings'];assert saved['duiRequireHybridKeyShare'] is False
  for fp in ('hellochrome_120','chrome'):
   reality_node['stream']['realitySettings']['fingerprint']=fp;probe(reality_node)
  print(json.dumps({'restart_reconnect':len(nodes),'reality_save_reload_switches':4,'reality_live_positive_negative_probes':11,'keys_preserved':True,'default_legacy_preserved':True}),flush=True)
 finally:
  stop_panel(panel);httpd.shutdown();camo.shutdown()
