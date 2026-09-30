import os,json,socket,sqlite3,subprocess,tempfile,time,uuid,signal,shutil,urllib.request,urllib.parse,http.cookiejar,threading,http.server,struct,concurrent.futures
from pathlib import Path
os.umask(0o077)
def port():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
TEST_UID = 65534 if os.geteuid()==0 else os.getuid()
TEST_GID = 65534 if os.geteuid()==0 else os.getgid()
def unprivileged():
 if os.geteuid()==0:os.setgroups([]);os.setgid(TEST_GID);os.setuid(TEST_UID)
class Handler(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  data=b'x'*(1024*1024) if self.path=='/rate' else b'dui-native-protocol-test';self.send_response(200);self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
 def do_POST(self):
  remaining=int(self.headers['Content-Length'])
  while remaining:
   chunk=self.rfile.read(min(65536,remaining))
   if not chunk:break
   remaining-=len(chunk)
  self.send_response(200 if remaining==0 else 400);self.end_headers();self.wfile.write(b'uploaded')
 def log_message(self,*args):pass
httpd=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler);threading.Thread(target=httpd.serve_forever,daemon=True).start()
echo=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);echo.bind(('127.0.0.1',0));echo.settimeout(.5);running=True
def udp_echo():
 while running:
  try:data,addr=echo.recvfrom(65535);echo.sendto(data,addr)
  except socket.timeout:pass
  except OSError:
   if not running:break
   raise
threading.Thread(target=udp_echo,daemon=True).start()
with tempfile.TemporaryDirectory(prefix='dui-native-panel-test-') as tmp:
 root=Path(tmp);os.chown(root,TEST_UID,TEST_GID)
 for name in ('db','bin','logs'):
  d=root/name;d.mkdir();os.chown(d,TEST_UID,TEST_GID)
 for src,dst in [(Path(os.environ['DUI_TEST_PANEL']),root/'x-ui'),(Path(os.environ['DUI_TEST_CORE']),root/'bin/xray-linux-amd64')]:shutil.copy2(src,dst);dst.chmod(0o755)
 subprocess.run(['openssl','req','-x509','-newkey','ec','-pkeyopt','ec_paramgen_curve:prime256v1','-nodes','-days','1','-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost','-keyout',str(root/'key.pem'),'-out',str(root/'cert.pem')],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 for name in ('key.pem','cert.pem'):os.chown(root/name,TEST_UID,TEST_GID)
 env=os.environ.copy();env.update(XUI_DB_FOLDER=str(root/'db'),XUI_BIN_FOLDER=str(root/'bin'),XUI_LOG_FOLDER=str(root/'logs'),XUI_LOG_LEVEL='error')
 pp=port();ap=port();password=uuid.uuid4().hex;panel=None;probe_proc=None
 def cli(args):
  p=subprocess.run([str(root/'x-ui'),*args],cwd=root,env=env,preexec_fn=unprivileged,capture_output=True,timeout=30);assert p.returncode==0,'test panel CLI failed'
 cli(['setting','-username','dui-smoke','-password',password,'-port',str(pp),'-webBasePath','/','-listenIP','127.0.0.1'])
 db=root/'db/x-ui.db'
 def wait_db(sql,args,predicate,message):
  for attempt in range(60):
   with sqlite3.connect(db) as conn:row=conn.execute(sql,args).fetchone()
   if row is not None and predicate(row):return row
   time.sleep(.5)
  raise AssertionError(message+': '+str(row))
 template={'log':{'loglevel':'none','access':'none','error':'none'},'api':{'tag':'api','services':['HandlerService','StatsService']},'stats':{},'inbounds':[{'tag':'api','listen':'127.0.0.1','port':ap,'protocol':'tunnel','settings':{'address':'127.0.0.1'}}],'outbounds':[{'tag':'direct','protocol':'freedom','settings':{'finalRules':[{'action':'allow','ip':['127.0.0.0/8']}]}}],'routing':{'rules':[{'type':'field','inboundTag':['api'],'outboundTag':'api'}]},'policy':{'levels':{'0':{'statsUserUplink':True,'statsUserDownlink':True}},'system':{'statsInboundUplink':True,'statsInboundDownlink':True}}}
 with sqlite3.connect(db) as conn:conn.execute('INSERT INTO settings(key,value) VALUES(?,?)',('xrayTemplateConfig',json.dumps(template)))
 def waitport(p,number):
  for _ in range(100):
   if p.poll() is not None:raise AssertionError('test process exited')
   try:
    with socket.create_connection(('127.0.0.1',number),timeout=.1):return
   except OSError:time.sleep(.1)
  raise AssertionError('test process did not listen')
 def start_panel():
  p=subprocess.Popen([str(root/'x-ui')],cwd=root,env=env,preexec_fn=unprivileged,start_new_session=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL);waitport(p,pp);waitport(p,ap);return p
 def stop(p,group=False):
  if p is None:return
  try:
   if group:os.killpg(p.pid,signal.SIGTERM)
   else:p.terminate()
  except ProcessLookupError:pass
  try:p.wait(timeout=8)
  except subprocess.TimeoutExpired:
   if group:os.killpg(p.pid,signal.SIGKILL)
   else:p.kill()
   p.wait()
 def login():
  global opener
  opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()));assert request('/login',{'username':'dui-smoke','password':password})['success']
 def request(path,data=None):
  body=None if data is None else urllib.parse.urlencode(data).encode()
  with opener.open('http://127.0.0.1:'+str(pp)+path,body,timeout=30) as r:return json.load(r)
 def api(path,data=None):
  r=request('/dui/api/'+path,data);assert r.get('success'), 'isolated API failed: '+path+' '+str(r.get('msg',''));return r.get('obj')
 def coreapi(args):
  r=subprocess.run([str(root/'bin/xray-linux-amd64'),'api',*args,'--server=127.0.0.1:'+str(ap)],capture_output=True,text=True,timeout=12);assert r.returncode==0,'test core API failed';return r.stdout
 def http_probe(cp,auth=None,scheme='socks5h',expected=True):
  proxy=scheme+'://'+(('test:testpass@') if auth else '')+'127.0.0.1:'+str(cp)
  r=subprocess.run(['curl','--noproxy','','-sS','--max-time','3','--proxy',proxy,'http://127.0.0.1:'+str(httpd.server_port)+'/'],capture_output=True,timeout=5)
  assert (r.returncode==0 and r.stdout==b'dui-native-protocol-test')==expected,'HTTP proxy result mismatch: '+scheme+' expected='+str(expected)
 def udp_probe(cp):
  with socket.create_connection(('127.0.0.1',cp),timeout=3) as control:
   control.sendall(b'\x05\x01\x00');assert control.recv(2)==b'\x05\x00'
   control.sendall(b'\x05\x03\x00\x01'+socket.inet_aton('127.0.0.1')+b'\x00\x00');reply=control.recv(64);assert reply[:2]==b'\x05\x00'
   relay=(socket.inet_ntoa(reply[4:8]),int.from_bytes(reply[8:10],'big'))
   with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as packet:
    packet.settimeout(4);payload=b'native-udp-'+uuid.uuid4().bytes
    packet.sendto(b'\x00\x00\x00\x01'+socket.inet_aton('127.0.0.1')+echo.getsockname()[1].to_bytes(2,'big')+payload,relay)
    response,_=packet.recvfrom(1024);assert response[10:]==payload,'UDP echo mismatch'
 def hy_client(sp,auth):
  cp=port();cfg={'log':{'loglevel':'none'},'inbounds':[{'listen':'127.0.0.1','port':cp,'protocol':'socks','settings':{'auth':'noauth','udp':True,'ip':'127.0.0.1'}}],'outbounds':[{'protocol':'hysteria','settings':{'version':2,'address':'127.0.0.1','port':sp},'streamSettings':{'network':'hysteria','security':'tls','hysteriaSettings':{'version':2,'auth':auth},'tlsSettings':{'serverName':'localhost','allowInsecure':True,'alpn':['h3']}}}]}
  path=root/'probe.json';path.write_text(json.dumps(cfg));os.chown(path,TEST_UID,TEST_GID)
  p=subprocess.Popen([str(root/'bin/xray-linux-amd64'),'run','-config',str(path)],cwd=root,env=env,preexec_fn=unprivileged,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL);waitport(p,cp);return p,cp
 try:
  panel=start_panel();login()
  sp=port();auth='test:/?%'+uuid.uuid4().hex;email='dui-hy-test'
  settings={'version':2,'clients':[{'auth':auth,'email':email,'enable':True,'speedLimitMbps':3,'subId':'dui-hy-sub'}]}
  stream={'network':'hysteria','security':'tls','hysteriaSettings':{'version':2},'tlsSettings':{'serverName':'localhost','alpn':['h3'],'certificates':[{'certificateFile':str(root/'cert.pem'),'keyFile':str(root/'key.pem')}],'settings':{'allowInsecure':True}}}
  form={'listen':'127.0.0.1','port':sp,'protocol':'hysteria','enable':'true','remark':'isolated Hy2','settings':json.dumps(settings),'streamSettings':json.dumps(stream),'sniffing':'{}'}
  created=api('inbounds/add',form);hid=created['id'];htag=created['tag']
  probe_proc,cp=hy_client(sp,auth);http_probe(cp);udp_probe(cp)
  raw=coreapi(['inbounduser','-tag='+htag,'-email='+email]);assert json.loads(raw)['users'][0]['level']==(1<<31)|375000
  print(json.dumps({'hysteria_panel_save_hot_add_TCP_UDP':True,'hot_rate_preserved':True}),flush=True)
  def rate_probe(upload):
   args=['curl','--noproxy','','-sS','--max-time','20','--proxy','socks5h://127.0.0.1:'+str(cp),'-o','/dev/null','-w','%{http_code}']
   if upload:args+=['--data-binary','@-']
   args+=['http://127.0.0.1:'+str(httpd.server_port)+'/rate']
   start=time.monotonic();r=subprocess.run(args,input=b'x'*(1024*1024) if upload else None,capture_output=True,timeout=23)
   assert r.returncode==0 and r.stdout==b'200','rate transfer failed';return time.monotonic()-start
  with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
   times=list(pool.map(rate_probe,[False,False,True,True]))
  assert 4.5<=max(times[:2])<=15 and 4.5<=max(times[2:])<=15,'aggregate directional rate did not match 3 Mbps'
  assert max(times)<10,'upload and download unexpectedly shared one 3 Mbps budget'
  print(json.dumps({'aggregate_3Mbps_two_uploads_two_downloads_seconds':[round(t,2) for t in times]}),flush=True)
  for protocol,authmode in [('mixed','password'),('mixed','noauth')]:
   mp=port();mixed={'listen':'127.0.0.1','port':mp,'protocol':protocol,'enable':'true','remark':'isolated Mixed','settings':json.dumps({'auth':authmode,'accounts':[{'user':'test','pass':'testpass'}] if authmode=='password' else [],'udp':True,'ip':'127.0.0.1'}),'streamSettings':'{}','sniffing':'{}'}
   api('inbounds/add',mixed)
   for scheme in ('socks5h','http'):
    http_probe(mp,auth=authmode=='password',scheme=scheme)
    if authmode=='password':http_probe(mp,scheme=scheme,expected=False)
   if authmode=='noauth':udp_probe(mp)
  bad=dict(form);bad['streamSettings']=json.dumps({'network':'tcp','security':'none'});r=request('/dui/api/inbounds/update/'+str(hid),bad);assert not r['success'];http_probe(cp)
  with socket.socket() as occupied:
   occupied.bind(('127.0.0.1',0));occupied.listen()
   bad_mixed=dict(mixed);bad_mixed['port']=occupied.getsockname()[1]
   r=request('/dui/api/inbounds/add',bad_mixed);assert not r['success'],'occupied port was saved as working';http_probe(cp)
  tun={'listen':'','port':0,'protocol':'tun','enable':'false','remark':'isolated TUN','settings':json.dumps({'name':'duitest0','mtu':1500,'gateway':['172.31.255.1/30'],'autoSystemRoutingTable':[],'autoOutboundsInterface':''}),'streamSettings':'{}','sniffing':'{}'}
  t=api('inbounds/add',tun);assert t['port']==0 and t['tag']=='inbound-tun-duitest0'
  print(json.dumps({'mixed_SOCKS_HTTP_auth_cases':6,'mixed_UDP':True,'invalid_Hy_edit_keeps_service':True,'TUN_disabled_save_no_routes':True}),flush=True)
  # Reuse the established QUIC client: a fresh client alone would miss stale
  # authentication retained by the server's existing QUIC connection.
  disabled=dict(settings['clients'][0]);disabled['enable']=False
  api('inbounds/updateClient/'+urllib.parse.quote(auth,safe=''),{'id':hid,'settings':json.dumps({'clients':[disabled]})})
  time.sleep(1.2);http_probe(cp,expected=False)
  print(json.dumps({'hysteria_existing_QUIC_revoked':True}),flush=True)
  # Saving the parent inbound must not accidentally re-enable a disabled user.
  saved=api('inbounds/get/'+str(hid));parent=dict(form);parent['settings']=saved['settings']
  api('inbounds/update/'+str(hid),parent);http_probe(cp,expected=False)
  disabled['enable']=True
  api('inbounds/updateClient/'+urllib.parse.quote(auth,safe=''),{'id':hid,'settings':json.dumps({'clients':[disabled]})})
  stop(probe_proc);probe_proc=None;probe_proc,cp=hy_client(sp,auth);http_probe(cp);udp_probe(cp)
  second={'auth':'second-'+uuid.uuid4().hex,'email':'dui-hy-second','enable':True,'speedLimitMbps':5}
  api('inbounds/addClient',{'id':hid,'settings':json.dumps({'clients':[second]})})
  assert len(json.loads(coreapi(['inbounduser','-tag='+htag]))['users'])==2
  api('inbounds/'+str(hid)+'/delClient/'+urllib.parse.quote(second['auth'],safe=''),{})
  stop(probe_proc);probe_proc=None;stop(panel,True);panel=None
  panel=start_panel();login();probe_proc,cp=hy_client(sp,auth);http_probe(cp);udp_probe(cp)
  saved=api('inbounds/get/'+str(hid));assert json.loads(saved['settings'])['clients'][0]['auth']==auth
  print(json.dumps({'hysteria_add_delete_enable_restart_TCP_UDP':True}),flush=True)
  # Optional helper suite, using only files explicitly supplied by the caller.
  if os.environ.get('DUI_TEST_HELPER_BIN'):
   hb=Path(os.environ['DUI_TEST_HELPER_BIN'])
   for name in ('tuic-server-linux-amd64','mtg-linux-amd64'):
    shutil.copy2(hb/name,root/'bin'/name);(root/'bin'/name).chmod(0o755)
   tc={'id':str(uuid.uuid4()),'password':uuid.uuid4().hex,'email':'dui-tuic-test','enable':True,'speedLimitMbps':0,'subId':'tuic-test','expiryTime':-86400000}
   ts={'clients':[tc],'certificate':str(root/'cert.pem'),'private_key':str(root/'key.pem'),'sni':'localhost','alpn':['h3'],'congestion_control':'bbr','zero_rtt_handshake':False}
   tp=port();tf={'listen':'127.0.0.1','port':tp,'protocol':'tuic','enable':'true','settings':json.dumps(ts),'streamSettings':'{}','sniffing':'{}'}
   ti=api('inbounds/add',tf)['id']
   spc=port();scfg={'log':{'disabled':True},'inbounds':[{'type':'socks','listen':'127.0.0.1','listen_port':spc}],'outbounds':[{'type':'tuic','server':'127.0.0.1','server_port':tp,'uuid':tc['id'],'password':tc['password'],'congestion_control':'bbr','tls':{'enabled':True,'server_name':'localhost','insecure':True,'alpn':['h3']}}]}
   sf=root/'tuic-client.json';sf.write_text(json.dumps(scfg));os.chown(sf,TEST_UID,TEST_GID)
   shutil.copy2(hb/'sing-box-test',root/'sing-box-test');(root/'sing-box-test').chmod(0o755)
   sb=None
   try:
    sb=subprocess.Popen([str(root/'sing-box-test'),'run','-c',str(sf)],cwd=root,preexec_fn=unprivileged,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    waitport(sb,spc);http_probe(spc);udp_probe(spc)
    # The statistics job is registered five seconds after each panel start,
    # then runs every ten seconds; use a bounded condition, not a shorter sleep.
    for attempt in range(30):
     with sqlite3.connect(db) as conn:
      expiry,last=conn.execute('SELECT expiry_time,last_online FROM client_traffics WHERE email=?',(tc['email'],)).fetchone()
     if expiry>int(time.time()*1000) and last>0:break
     time.sleep(1)
    assert expiry>int(time.time()*1000) and last>0,'TUIC first-use expiry/activity missing: '+str((expiry,last))
    tc['expiryTime']=expiry
    rejected=dict(tc);rejected['speedLimitMbps']=100
    response=request('/dui/api/inbounds/updateClient/'+tc['id'],{'id':ti,'settings':json.dumps({'clients':[rejected]})});assert not response['success'],'unsupported TUIC user limit accepted'
    http_probe(spc)
    tc['enable']=False;api('inbounds/updateClient/'+tc['id'],{'id':ti,'settings':json.dumps({'clients':[tc]})});http_probe(spc,expected=False)
    tc['enable']=True;api('inbounds/updateClient/'+tc['id'],{'id':ti,'settings':json.dumps({'clients':[tc]})});http_probe(spc);udp_probe(spc)
    bad=dict(tf);bs=dict(ts);bs['private_key']='/nonexistent-key';bad['settings']=json.dumps(bs)
    response=request('/dui/api/inbounds/update/'+str(ti),bad);assert not response['success'];http_probe(spc)
    stop(panel,True);panel=None;panel=start_panel();login()
    # The core API is ready before its helpers. A pre-existing QUIC client may
    # also need to discard its old session after an intentional server restart.
    recovered=False;restart_start=time.monotonic()
    for attempt in range(8):
     try:http_probe(spc);udp_probe(spc);recovered=True;break
     except AssertionError:time.sleep(.5)
    assert recovered,'TUIC client did not recover after panel restart'
    print(json.dumps({'TUIC_restart_recovery_seconds':round(time.monotonic()-restart_start,2)}),flush=True)
    api('inbounds/del/'+str(ti),{});http_probe(spc,expected=False)
    print(json.dumps({'TUIC_v5_real_TCP_UDP_disable_enable_restart_delete':True,'TUIC_bad_config_keeps_runtime':True}),flush=True)
   finally:stop(sb)
   # Telegram FakeTLS handshake: authenticate ClientHello and independently
   # verify the server HMAC over all three response records. This verifies the
   # secret on the wire; it is not a TCP-port-only readiness probe.
   def fake_tls_probe(server_port,credential):
    import ssl,hmac,hashlib
    raw=bytes.fromhex(credential);key=raw[1:17];domain=raw[17:].decode()
    ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT);ctx.check_hostname=False;ctx.verify_mode=ssl.CERT_NONE
    inp,out=ssl.MemoryBIO(),ssl.MemoryBIO();tls=ctx.wrap_bio(inp,out,server_side=False,server_hostname=domain)
    try:tls.do_handshake()
    except ssl.SSLWantReadError:pass
    hello=bytearray(out.read());hello[11:43]=bytes(32)
    digest=bytearray(hmac.new(key,hello,hashlib.sha256).digest());ts=int(time.time()).to_bytes(4,'little')
    for j in range(4):digest[28+j]^=ts[j]
    hello[11:43]=digest
    try:
     with socket.create_connection(('127.0.0.1',server_port),timeout=3) as conn:
      conn.settimeout(3);conn.sendall(hello)
      def readn(n):
       data=b''
       while len(data)<n:
        chunk=conn.recv(n-len(data))
        if not chunk:raise EOFError()
        data+=chunk
       return data
      response=bytearray()
      for record_type in (22,20,23):
       header=readn(5)
       if header[0]!=record_type:return False
       response+=header+readn(int.from_bytes(header[3:5],'big'))
      received=bytes(response[11:43]);response[11:43]=bytes(32)
      return hmac.compare_digest(received,hmac.new(key,bytes(digest)+response,hashlib.sha256).digest())
    except (OSError,EOFError):return False

   secret='ee'+uuid.uuid4().hex+'www.cloudflare.com'.encode().hex()
   mc={'id':str(uuid.uuid4()),'secret':secret,'email':'dui-mt-test','enable':True,'speedLimitMbps':0}
   mp=port();mf={'listen':'127.0.0.1','port':mp,'protocol':'mtproto','enable':'true','settings':json.dumps({'clients':[mc]}),'streamSettings':'{}','sniffing':'{}'}
   mi=api('inbounds/add',mf)['id'];waitport(panel,mp)
   assert fake_tls_probe(mp,secret),'MTProto valid FakeTLS rejected'
   wrong='ee'+uuid.uuid4().hex+'www.cloudflare.com'.encode().hex()
   assert not fake_tls_probe(mp,wrong),'MTProto wrong FakeTLS accepted'
   print(json.dumps({'MTProto_real_FakeTLS_auth_valid_wrong':True}),flush=True)
   def mtproto_probe(credential):
    import ssl,base64
    ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT);ctx.check_hostname=False;ctx.verify_mode=ssl.CERT_NONE
    inp,out=ssl.MemoryBIO(),ssl.MemoryBIO();tls=ctx.wrap_bio(inp,out,server_side=False,server_hostname=bytes.fromhex(credential)[17:].decode())
    try:tls.do_handshake()
    except ssl.SSLWantReadError:pass
    data={'address':'127.0.0.1:'+str(mp),'secret':credential,'hello':base64.b64encode(out.read()).decode(),'dc':2}
    result=subprocess.run([os.environ['DUI_TEST_MTPROTO_CLIENT']],input=json.dumps(data),capture_output=True,text=True,timeout=20,preexec_fn=unprivileged)
    assert result.returncode==0,'MTProto test client exited'
    reply=json.loads(result.stdout);assert reply['ok'],'MTProto data plane: '+reply.get('error','')
   if os.environ.get('DUI_TEST_MTPROTO_CLIENT'):
    mtproto_probe(secret)
    print(json.dumps({'MTProto_real_Telegram_req_pq_resPQ':True}),flush=True)
   config_path=root/'bin/mtproto'/('mtg-'+str(mi)+'.toml')
   assert secret in config_path.read_text(),'MTProto auth not emitted'
   mc['enable']=False;api('inbounds/updateClient/'+urllib.parse.quote(secret,safe=''),{'id':mi,'settings':json.dumps({'clients':[mc]})})
   try:
    with socket.create_connection(('127.0.0.1',mp),timeout=.3):raise AssertionError('MTProto disabled user kept listener alive')
   except OSError:pass
   mc['enable']=True;api('inbounds/updateClient/'+secret,{'id':mi,'settings':json.dumps({'clients':[mc]})});waitport(panel,mp)
   stop(panel,True);panel=None;panel=start_panel();login();waitport(panel,mp)
   assert fake_tls_probe(mp,secret),'MTProto secret broken after restart'
   if os.environ.get('DUI_TEST_MTPROTO_CLIENT'):mtproto_probe(secret)
   print(json.dumps({'MTProto_panel_start_disable_enable_restart':True,'MTProto_data_protocol_probe_pending':not bool(os.environ.get('DUI_TEST_MTPROTO_CLIENT'))}),flush=True)
   if os.environ.get('DUI_TEST_MTPROTO_CLIENT'):
    second_secret='ee'+uuid.uuid4().hex+'www.cloudflare.com'.encode().hex()
    second_mc={'id':str(uuid.uuid4()),'secret':second_secret,'email':'dui-mt-second','enable':True}
    api('inbounds/addClient',{'id':mi,'settings':json.dumps({'clients':[second_mc]})})
    assert fake_tls_probe(mp,second_secret)
    # This short exchange ends before the first traffic scrape. Its bytes must
    # still be attributed to this user, including after a process restart.
    wait_db('SELECT up,down FROM client_traffics WHERE email=?',(mc['email'],),lambda row:row[0]>0 and row[1]>0,'first MTProto traffic scrape discarded bytes')
    mc['totalGB']=1
    api('inbounds/updateClient/'+secret,{'id':mi,'settings':json.dumps({'clients':[mc]})})
    api('inbounds/'+str(mi)+'/resetClientTraffic/'+mc['email'],{})
    mtproto_probe(secret)
    assert not fake_tls_probe(mp,secret),'MTProto exhausted quota still authenticates'
    assert fake_tls_probe(mp,second_secret),'one user quota blocked another user'
    wait_db('SELECT enable FROM client_traffics WHERE email=?',(mc['email'],),lambda row:row[0]==0,'MTProto quota not disabled')
    api('inbounds/'+str(mi)+'/resetClientTraffic/'+mc['email'],{})
    assert fake_tls_probe(mp,secret),'quota reset left mtg blocked'
    mc['totalGB']=1000000;mc['expiryTime']=int(time.time()*1000)+1500
    api('inbounds/updateClient/'+secret,{'id':mi,'settings':json.dumps({'clients':[mc]})});time.sleep(2)
    assert not fake_tls_probe(mp,secret),'expired MTProto user still authenticates'
    assert fake_tls_probe(mp,second_secret)
    mc['reset']=1;mc['expiryTime']=int(time.time()*1000)-1000
    api('inbounds/updateClient/'+secret,{'id':mi,'settings':json.dumps({'clients':[mc]})})
    wait_db('SELECT expiry_time FROM client_traffics WHERE email=?',(mc['email'],),lambda row:row[0]>int(time.time()*1000),'MTProto auto renewal missing')
    assert fake_tls_probe(mp,secret),'automatic renewal did not reapply helper secret'
    api('server/stopXrayService',{})
    api('inbounds/'+str(mi)+'/resetClientTraffic/'+mc['email'],{})
    api('server/restartXrayService',{});waitport(panel,mp)
    assert fake_tls_probe(mp,secret),'offline quota reset lost on restart'
    mtproto_probe(secret);mtproto_probe(second_secret)
    mc['reset']=0;mc['expiryTime']=int(time.time()*1000)-1000
    api('inbounds/updateClient/'+secret,{'id':mi,'settings':json.dumps({'clients':[mc]})})
    wait_db('SELECT enable FROM client_traffics WHERE email=?',(mc['email'],),lambda row:row[0]==0,'MTProto expired user not depleted')
    api('inbounds/delDepletedClients/'+str(mi),{})
    remaining=next(x for x in api('inbounds/list') if x['id']==mi)
    assert [c['secret'] for c in json.loads(remaining['settings'])['clients']]==[second_secret]
    assert not fake_tls_probe(mp,secret),'depleted MTProto user still authenticates'
    mtproto_probe(second_secret)
    second_mc['expiryTime']=int(time.time()*1000)-1000
    api('inbounds/updateClient/'+second_secret,{'id':mi,'settings':json.dumps({'clients':[second_mc]})})
    wait_db('SELECT enable FROM client_traffics WHERE email=?',(second_mc['email'],),lambda row:row[0]==0,'MTProto final expired user not depleted')
    api('inbounds/delDepletedClients/'+str(mi),{})
    assert not any(x['id']==mi for x in api('inbounds/list')),'empty depleted helper inbound was retained'
    assert not fake_tls_probe(mp,second_secret),'deleting final depleted user left helper running'
    print(json.dumps({'MTProto_two_users_quota_expiry_auto_renew_offline_reset_and_traffic':True}),flush=True)

  if os.environ.get('DUI_TEST_AWG_CLIENT'):
   # Run this section inside an isolated network namespace with 198.18.0.1
   # bound to a dummy interface. No host routes, kernel TUN or firewall writes.
   import base64
   tcp_echo=socket.socket();tcp_echo.bind(('198.18.0.1',0));tcp_echo.listen();tcp_echo.settimeout(.5)
   awg_udp=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);awg_udp.bind(('198.18.0.1',0));awg_udp.settimeout(.5)
   awg_running=True
   def awg_tcp_echo():
    while awg_running:
     try:conn,_=tcp_echo.accept()
     except socket.timeout:continue
     except OSError:return
     def relay(c):
      with c:
       c.settimeout(5)
       try:
        while True:
         data=c.recv(4096)
         if not data:return
         c.sendall(data)
       except OSError:pass
     threading.Thread(target=relay,args=(conn,),daemon=True).start()
   def awg_udp_echo():
    while awg_running:
     try:data,addr=awg_udp.recvfrom(4096);awg_udp.sendto(data,addr)
     except socket.timeout:pass
     except OSError:return
   threading.Thread(target=awg_tcp_echo,daemon=True).start();threading.Thread(target=awg_udp_echo,daemon=True).start()
   awgc=subprocess.Popen([os.environ['DUI_TEST_AWG_CLIENT']],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True,preexec_fn=unprivileged)
   def awg_command(**kw):
    import select
    awgc.stdin.write(json.dumps(kw)+'\n');awgc.stdin.flush()
    ready,_,_=select.select([awgc.stdout],[],[],8);assert ready,'AWG test client timeout'
    return json.loads(awgc.stdout.readline())
   def awg_probe(expected=True):
    for network,dest in [('tcp',tcp_echo.getsockname()),('udp',awg_udp.getsockname())]:
     started=time.monotonic()
     while True:
      result=awg_command(op='probe',network=network,destination=dest[0]+':'+str(dest[1]))
      if result['ok'] or not expected or time.monotonic()-started>=40:break
      time.sleep(.5)
     assert result['ok']==expected,'AWG '+network+' expected='+str(expected)+' '+result.get('error','')
     if expected and time.monotonic()-started>3:print(json.dumps({'AWG_same_client_recovery_seconds':round(time.monotonic()-started,2)}),flush=True)
   try:
    sk=awg_command(op='keys');ck=awg_command(op='keys');ac={'id':str(uuid.uuid4()),'email':'dui-awg-test','enable':True,'publicKey':ck['public'],'privateKey':ck['private'],'allowedIPs':['10.8.0.2/32'],'totalGB':0}
    aserver={'privateKey':sk['private'],'publicKey':sk['public'],'subnetIp':'10.8.0.0','subnetCidr':24,'mtu':1280,'jc':4,'jmin':40,'jmax':70,'s1':20,'s2':30,'s3':20,'s4':20}
    asp=port();aset={'server':aserver,'clients':[ac]};af={'listen':'127.0.0.1','port':asp,'protocol':'amneziawg','enable':'true','settings':json.dumps(aset),'streamSettings':'{}','sniffing':'{}'}
    ai=api('inbounds/add',af);aid=ai['id'];atag=ai['tag']
    conf='private_key='+base64.b64decode(ck['private']).hex()+'\njc=4\njmin=40\njmax=70\ns1=20\ns2=30\ns3=20\ns4=20\npublic_key='+base64.b64decode(sk['public']).hex()+'\nendpoint=127.0.0.1:'+str(asp)+'\nallowed_ip=0.0.0.0/0\n'
    assert awg_command(op='setup',address='10.8.0.2',config=conf)['ok'];awg_probe()
    # Native CRUD must also reserve the hidden relay port.
    collision=dict(mixed);collision['port']=65101+(aid-1)%435
    assert not request('/dui/api/inbounds/add',collision)['success'];awg_probe()
    ac['enable']=False;api('inbounds/updateClient/'+ac['id'],{'id':aid,'settings':json.dumps({'clients':[ac]})});awg_probe(False)
    ac['enable']=True;api('inbounds/updateClient/'+ac['id'],{'id':aid,'settings':json.dumps({'clients':[ac]})});awg_probe()
    # Exercise the periodic quota path through persisted counters, not by
    # calling an internal filtering function. Existing tunnel must be revoked.
    ac['totalGB']=1000000;api('inbounds/updateClient/'+ac['id'],{'id':aid,'settings':json.dumps({'clients':[ac]})})
    with sqlite3.connect(db) as conn:conn.execute('UPDATE client_traffics SET up=1000001 WHERE email=?',(ac['email'],))
    time.sleep(12);awg_probe(False)
    api('inbounds/'+str(aid)+'/resetClientTraffic/'+ac['email'],{});awg_probe()
    # Failed parent-port change must restore both peer and the old SOCKS tag.
    occupied=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);occupied.bind(('0.0.0.0',0))
    try:
     bad=dict(af);bad['port']=occupied.getsockname()[1];bad['settings']=json.dumps({'server':aserver,'clients':[ac]})
     response=request('/dui/api/inbounds/update/'+str(aid),bad);assert not response['success'],'occupied AWG port accepted'
    finally:occupied.close()
    awg_probe()
    saved=api('inbounds/get/'+str(aid));assert saved['port']==asp
    stop(panel,True);panel=None;panel=start_panel();login();time.sleep(.7)
    # Keep the same real client through restart, allowing WireGuard handshake retry.
    recovered=False
    for _ in range(6):
     try:awg_probe();recovered=True;break
     except AssertionError:time.sleep(1)
    assert recovered,'AmneziaWG client did not recover after panel restart'
    api('inbounds/'+str(aid)+'/delClient/'+ac['id'],{});awg_probe(False)
    api('inbounds/del/'+str(aid),{})
    print(json.dumps({'AmneziaWG_panel_TCP_UDP_disable_quota_reset_restart_delete':True,'AWG_reserved_bridge_and_failed_port_rollback':True}),flush=True)
   finally:
    stop(awgc);awg_running=False;tcp_echo.close();awg_udp.close()

 finally:stop(probe_proc);stop(panel,True)
running=False;echo.close();httpd.shutdown()
