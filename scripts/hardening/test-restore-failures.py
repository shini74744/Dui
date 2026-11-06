import socket,json,sqlite3,urllib.request,urllib.error,time,hashlib
from fixture import Fixture
results=[]
def upload(f,op,body):
 boundary='dui-audit-boundary'
 data=('--'+boundary+'\r\nContent-Disposition: form-data; name="db"; filename="audit.db"\r\nContent-Type: application/octet-stream\r\n\r\n').encode()+body+('\r\n--'+boundary+'--\r\n').encode()
 req=urllib.request.Request(f.base+'/dui/api/server/importDB',data=data,headers={'Content-Type':'multipart/form-data; boundary='+boundary,'X-Requested-With':'XMLHttpRequest'})
 try:
  with op.open(req,timeout=90) as r:return r.status,json.load(r)
 except urllib.error.HTTPError as e:return e.code,{'success':False,'error':'HTTP error'}
 except Exception as e:return 0,{'success':False,'error':type(e).__name__}
for kind in ('corrupt-sqlite','invalid-core-config'):
 with Fixture('restore-'+kind) as f:
  op=f.login();reply,form=f.add(op,email='must-survive');assert reply[1].get('success')
  with sqlite3.connect(f.db) as c:original=c.execute('select value from settings where key="xrayTemplateConfig"').fetchone()[0]
  if kind=='corrupt-sqlite':body=b'SQLite format 3\0'+b'\0'*200
  else:
   path=f.root/'uploaded.db'
   with sqlite3.connect(f.db) as src,sqlite3.connect(path) as dst:src.backup(dst);dst.execute('update settings set value=? where key=?',('{','xrayTemplateConfig'));dst.commit()
   body=path.read_bytes()
  status,result=upload(f,op,body);time.sleep(.2)
  try:list_status,list_response,_=f.request('/dui/api/inbounds/list',opener=op);list_ok=list_response.get('success',False)
  except Exception:list_status=0;list_ok=False
  with sqlite3.connect(f.db) as c:
   integrity=c.execute('pragma integrity_check').fetchone()[0]
   current=c.execute('select value from settings where key="xrayTemplateConfig"').fetchone()
  assert result.get('success') is False and list_ok and current[0]==original and integrity=='ok' and f.listens(f.apiport) and f.listens(form['port']),result
  results.append({'test':kind,'upload_status':status,'upload_success':result.get('success'),'upload_message':result.get('msg') or result.get('error'),'subsequent_list_status':list_status,'subsequent_list_success':list_ok,'original_config_preserved':current is not None and current[0]==original,'database_integrity':integrity,'fallback_backup_exists':(f.root/'db/x-ui.db.backup').exists(),'core_api_running':f.listens(f.apiport),'business_listener_running':f.listens(form['port'])})

# Valid restore and activation failure after validation: keep a TCP port occupied.
for collision in (False,True):
 with Fixture('restore-activation-'+str(collision)) as f:
  op=f.login();reply,form=f.add(op,email='restore-original');assert reply[1]['success']
  blocker=socket.socket();blocker.bind(('127.0.0.1',0));blocker.listen()
  path=f.root/'uploaded.db'
  with sqlite3.connect(f.db) as src,sqlite3.connect(path) as dst:
   src.backup(dst);dst.execute("update inbounds set remark='restored-mark'")
   if collision:
    template=json.loads(dst.execute("select value from settings where key='xrayTemplateConfig'").fetchone()[0])
    template['inbounds'].append({'tag':'bind-collision','listen':'127.0.0.1','port':blocker.getsockname()[1],'protocol':'socks','settings':{'auth':'noauth'}})
    dst.execute("update settings set value=? where key='xrayTemplateConfig'",(json.dumps(template),))
   dst.commit()
  status,result=upload(f,op,path.read_bytes())
  assert result.get('success') is (not collision),result
  rows=f.inbounds(op);assert rows[0]['remark']==('restore-original' if collision else 'restored-mark'),rows
  backups=list((f.root/'db').glob('x-ui.db.backup-*'))
  assert len(backups)==1 and (backups[0].stat().st_mode & 0o777)==0o600
  assert f.listens(f.apiport) and f.listens(form['port']),'original business listener missing'
  blocker.close();results.append({'test':'activation_failure_rollback' if collision else 'valid_restore','success':True,'backup_retained':True,'business_running':True})
print(json.dumps(results,indent=2))
