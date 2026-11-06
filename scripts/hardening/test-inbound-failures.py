import concurrent.futures,json,sqlite3,time,threading
from fixture import Fixture,freeport
results=[]
with Fixture('inbound-concurrency') as f:
 op=f.login();barrier=threading.Barrier(16)
 def add(i):
  barrier.wait()
  response,form=f.add(op,enabled=False,email='concurrent-'+str(i))
  return {'status':response[0],'success':response[1].get('success'),'id':(response[1].get('obj') or {}).get('id'),'message':response[1].get('msg','')}
 with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:responses=list(pool.map(add,range(16)))
 rows=f.inbounds(op)
 assert len(rows)==16 and len({x['id'] for x in responses if x['success']})==16 and all(x['success'] for x in responses),responses
 results.append({'test':'concurrent_create','requests':16,'successes':sum(x['success'] is True for x in responses),'unique_success_ids':len({x['id'] for x in responses if x['success']}),'persisted_rows':len(rows),'responses':responses})
with Fixture('inbound-commit') as f:
 op=f.login()
 with sqlite3.connect(f.db) as c:mode=c.execute('pragma journal_mode').fetchone()[0]
 reader=sqlite3.connect(f.db);reader.execute('begin');reader.execute('select * from inbounds').fetchall()
 start=time.time()
 try:response,form=f.add(op,enabled=False,email='commit-lock')
 finally:reader.rollback();reader.close()
 rows=f.inbounds(op)
 assert response[1].get('success') is False and not rows
 results.append({'test':'commit_failure','journal_mode':mode,'seconds':round(time.time()-start,2),'api_success':response[1].get('success'),'saved_row_exists':any(x['remark']=='commit-lock' for x in rows),'message':response[1].get('msg','')})
with Fixture('inbound-invalid-edit') as f:
 op=f.login();response,form=f.add(op,email='valid-before-edit');assert response[1].get('success')
 row=response[1]['obj'];sp=form['port'];assert f.listens(sp)
 bad=dict(form);bad['streamSettings']=json.dumps({'network':'tcp','security':'tls','tlsSettings':{'certificates':[{'certificateFile':'/nonexistent-dui-audit-cert.pem','keyFile':'/nonexistent-dui-audit-key.pem'}]}})
 status,edited,_=f.request('/dui/api/inbounds/update/'+str(row['id']),bad,opener=op)
 after=f.inbounds(op);saved=next(x for x in after if x['id']==row['id'])
 status,restart,_=f.request('/dui/api/server/restartXrayService',{},opener=op)
 assert edited.get('success') is False and f.listens(sp) and json.loads(saved['streamSettings'])['security']=='none' and restart.get('success')
 results.append({'test':'invalid_ordinary_inbound_edit','update_success':edited.get('success'),'old_listener_survives':f.listens(sp),'invalid_config_saved':json.loads(saved['streamSettings'])['security']=='tls','core_restart_success':restart.get('success'),'core_api_still_running':f.listens(f.apiport),'update_message':edited.get('msg',''),'restart_message':restart.get('msg','')})

# Force commit failure after a real handler was installed, then after port change.
for change in (False,True):
 with Fixture('active-commit-'+str(change)) as f:
  op=f.login();old=None
  if change:
   response,form=f.add(op,email='active-original');assert response[1]['success'];old=response[1]['obj'];oldport=form['port']
  reader=sqlite3.connect(f.db);reader.execute('begin');reader.execute('select * from inbounds').fetchall()
  try:
   if change:
    form['port']=freeport();response=f.request('/dui/api/inbounds/update/'+str(old['id']),form,opener=op)
   else:response,form=f.add(op,email='active-commit')
  finally:reader.rollback();reader.close()
  assert response[1].get('success') is False,response[1]
  assert not f.listens(form['port']),'uncommitted listener survived'
  saved=f.inbounds(op)
  if change:assert saved[0]['port']==oldport and saved[0]['tag']==old['tag'] and f.listens(oldport)
  else:assert not saved
  results.append({'test':'active_edit_commit_rollback' if change else 'active_create_commit_rollback','database_and_runtime_rolled_back':True})
print(json.dumps(results,indent=2))
