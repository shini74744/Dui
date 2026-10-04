/* Update jobs live on the server; UI polling never owns the download. */
const DuiUpdateText = (() => {
 const keys=['title','panel','core','available','current','latest','check','checking','start','close','hint','unsupported','reconnecting','retry','reload','unknown','queued','connecting','downloading','verifying','preparing','installing','restarting','rolling_back','complete','rolled_back','failed'];
 const rows={
 'zh-CN':['软件更新','面板','内核','有更新','当前版本','最新版本','检查更新','正在检查','后台下载并更新','关闭','下载完成并通过校验后才替换。下载期间服务继续运行；安装时会短暂重启。关闭页面不影响下载。','此安装方式暂不支持网页更新，请使用安装脚本。','连接暂时中断，正在重连；后台任务继续运行。','重试','刷新页面','暂未获取','等待开始','正在连接下载','正在下载','校验完整性与版本','备份当前版本','正在安装','重启并验证服务','正在恢复原版本','更新完成','更新未通过，已恢复原版本','更新失败'],
 'zh-TW':['軟體更新','面板','核心','有更新','目前版本','最新版本','檢查更新','正在檢查','背景下載並更新','關閉','下載完成並通過校驗後才替換。下載時服務繼續運作；安裝時短暫重啟。關閉頁面不影響下載。','此安裝方式暫不支援網頁更新，請使用安裝腳本。','連線暫時中斷，正在重連；背景任務繼續執行。','重試','重新整理','尚未取得','等待開始','正在連線下載','正在下載','校驗完整性與版本','備份目前版本','正在安裝','重啟並驗證服務','正在還原原版本','更新完成','更新未通過，已還原原版本','更新失敗'],
 'en-US':['Software updates','Panel','Core','Update available','Installed','Latest','Check for updates','Checking','Download & update','Close','Replacement starts only after the full download passes verification. Services keep running during download and briefly restart for installation. You can close this page.','Web updates require a Linux systemd installation. Use the installer for this deployment.','Reconnecting; the background task continues.','Retry','Reload page','Not checked','Queued','Connecting','Downloading','Verifying integrity & version','Backing up','Installing','Restarting & checking','Restoring previous version','Update complete','Update rejected; previous version restored','Update failed'],
 'ja-JP':['ソフトウェア更新','パネル','コア','更新あり','現在','最新','更新を確認','確認中','ダウンロードして更新','閉じる','完全なダウンロードと検証後に置き換えます。ダウンロード中は稼働し、インストール時のみ再起動します。ページを閉じても継続します。','この環境は Web 更新に非対応です。インストーラーを使用してください。','再接続中。バックグラウンド処理は継続しています。','再試行','再読み込み','未取得','待機中','接続中','ダウンロード中','整合性とバージョンを検証中','バックアップ中','インストール中','再起動と確認中','旧版を復元中','更新完了','旧版に復元しました','更新失敗'],
 'ru-RU':['Обновления','Панель','Ядро','Есть обновление','Установлено','Последнее','Проверить','Проверка','Скачать и обновить','Закрыть','Замена только после полной загрузки и проверки. Службы работают во время загрузки и кратко перезапускаются при установке. Страницу можно закрыть.','Веб-обновления недоступны. Используйте установщик.','Переподключение. Фоновая задача продолжается.','Повторить','Обновить страницу','Нет данных','В очереди','Подключение','Загрузка','Проверка целостности и версии','Резервное копирование','Установка','Перезапуск и проверка','Восстановление','Обновлено','Предыдущая версия восстановлена','Ошибка обновления'],
 'vi-VN':['Cập nhật','Bảng điều khiển','Lõi','Có bản mới','Đã cài','Mới nhất','Kiểm tra','Đang kiểm tra','Tải và cập nhật','Đóng','Chỉ thay thế sau khi tải đủ và xác minh. Dịch vụ vẫn chạy khi tải, khởi động lại ngắn lúc cài. Có thể đóng trang.','Không hỗ trợ cập nhật web. Hãy dùng trình cài đặt.','Đang kết nối lại; tác vụ nền tiếp tục.','Thử lại','Tải lại trang','Chưa kiểm tra','Đang chờ','Đang kết nối','Đang tải','Đang xác minh','Đang sao lưu','Đang cài','Khởi động lại và kiểm tra','Đang khôi phục','Cập nhật xong','Đã khôi phục bản trước','Cập nhật lỗi'],
 'es-ES':['Actualizaciones','Panel','Núcleo','Actualización disponible','Instalado','Último','Buscar actualizaciones','Comprobando','Descargar y actualizar','Cerrar','Solo se reemplaza tras descargar y verificar todo. Los servicios siguen activos durante la descarga y se reinician brevemente al instalar. Puedes cerrar la página.','Usa el instalador; esta instalación no admite actualizaciones web.','Reconectando; la tarea continúa en segundo plano.','Reintentar','Recargar','Sin comprobar','En cola','Conectando','Descargando','Verificando integridad y versión','Creando copia','Instalando','Reiniciando y comprobando','Restaurando','Actualizado','Versión anterior restaurada','Error de actualización'],
 'id-ID':['Pembaruan','Panel','Inti','Pembaruan tersedia','Terpasang','Terbaru','Periksa pembaruan','Memeriksa','Unduh dan perbarui','Tutup','Penggantian hanya setelah unduhan lengkap diverifikasi. Layanan tetap berjalan saat mengunduh, lalu dimulai ulang sebentar. Halaman boleh ditutup.','Gunakan pemasang; pembaruan web tidak tersedia.','Menghubungkan kembali; tugas latar tetap berjalan.','Coba lagi','Muat ulang','Belum diperiksa','Mengantre','Menghubungkan','Mengunduh','Memverifikasi integritas dan versi','Mencadangkan','Memasang','Memulai ulang dan memeriksa','Memulihkan','Pembaruan selesai','Versi sebelumnya dipulihkan','Pembaruan gagal'],
 'uk-UA':['Оновлення','Панель','Ядро','Є оновлення','Встановлено','Останнє','Перевірити','Перевірка','Завантажити й оновити','Закрити','Заміна лише після повного завантаження та перевірки. Служби працюють під час завантаження й коротко перезапускаються при встановленні. Сторінку можна закрити.','Веб-оновлення недоступні. Скористайтеся інсталятором.','Повторне підключення; фонова задача триває.','Повторити','Оновити сторінку','Немає даних','У черзі','Підключення','Завантаження','Перевірка цілісності та версії','Резервування','Встановлення','Перезапуск і перевірка','Відновлення','Оновлено','Попередню версію відновлено','Помилка оновлення'],
 'tr-TR':['Güncellemeler','Panel','Çekirdek','Güncelleme var','Kurulu','En yeni','Güncellemeleri denetle','Denetleniyor','İndir ve güncelle','Kapat','Yalnızca tam indirme doğrulandıktan sonra değiştirilir. İndirme sırasında hizmetler çalışır, kurulumda kısa süre yeniden başlar. Sayfayı kapatabilirsiniz.','Web güncellemesi desteklenmiyor. Kurucuyu kullanın.','Yeniden bağlanılıyor; arka plan görevi sürüyor.','Tekrar dene','Sayfayı yenile','Denetlenmedi','Sırada','Bağlanılıyor','İndiriliyor','Bütünlük ve sürüm doğrulanıyor','Yedekleniyor','Kuruluyor','Yeniden başlatılıp denetleniyor','Geri yükleniyor','Güncellendi','Önceki sürüm geri yüklendi','Güncelleme başarısız'],
 'pt-BR':['Atualizações','Painel','Núcleo','Atualização disponível','Instalado','Mais recente','Verificar','Verificando','Baixar e atualizar','Fechar','A substituição só ocorre após baixar e verificar tudo. Os serviços continuam durante o download e reiniciam brevemente na instalação. Pode fechar a página.','Use o instalador; atualização web indisponível.','Reconectando; a tarefa em segundo plano continua.','Tentar novamente','Recarregar','Não verificado','Na fila','Conectando','Baixando','Verificando integridade e versão','Criando cópia','Instalando','Reiniciando e verificando','Restaurando','Atualização concluída','Versão anterior restaurada','Falha na atualização'],
 'ar-EG':['تحديثات البرنامج','اللوحة','النواة','تحديث متاح','المثبت','الأحدث','فحص التحديثات','جارٍ الفحص','تنزيل وتحديث','إغلاق','لا يتم الاستبدال إلا بعد اكتمال التنزيل والتحقق. تستمر الخدمات أثناء التنزيل وتُعاد لفترة قصيرة عند التثبيت. يمكنك إغلاق الصفحة.','تحديث الويب غير متاح؛ استخدم برنامج التثبيت.','إعادة الاتصال؛ تستمر المهمة في الخلفية.','إعادة المحاولة','إعادة تحميل','لم يُفحص','في الانتظار','جارٍ الاتصال','جارٍ التنزيل','التحقق من السلامة والإصدار','نسخ احتياطي','جارٍ التثبيت','إعادة التشغيل والتحقق','استعادة الإصدار السابق','اكتمل التحديث','تمت استعادة الإصدار السابق','فشل التحديث'],
 'fa-IR':['به‌روزرسانی','پنل','هسته','به‌روزرسانی موجود','نصب‌شده','جدیدترین','بررسی به‌روزرسانی','در حال بررسی','دانلود و به‌روزرسانی','بستن','جایگزینی فقط پس از دانلود کامل و تأیید انجام می‌شود. سرویس‌ها هنگام دانلود فعال می‌مانند و برای نصب کوتاه راه‌اندازی مجدد می‌شوند. می‌توانید صفحه را ببندید.','به‌روزرسانی وب پشتیبانی نمی‌شود؛ از نصب‌کننده استفاده کنید.','اتصال مجدد؛ کار پس‌زمینه ادامه دارد.','تلاش مجدد','بازخوانی','بررسی نشده','در صف','در حال اتصال','در حال دانلود','تأیید صحت و نسخه','پشتیبان‌گیری','در حال نصب','راه‌اندازی مجدد و بررسی','بازیابی نسخه قبل','به‌روزرسانی کامل شد','نسخه قبل بازیابی شد','به‌روزرسانی ناموفق']
 };
 const inlineRows={
  'zh-CN':['已是最新','检查失败，请重试'],
  'zh-TW':['已是最新','檢查失敗，請重試'],
  'en-US':['Up to date','Could not check for updates. Please retry.'],
  'ja-JP':['最新です','更新を確認できません。再試行してください。'],
  'ru-RU':['Обновлено','Не удалось проверить обновления. Повторите попытку.'],
  'vi-VN':['Đã cập nhật','Không thể kiểm tra. Vui lòng thử lại.'],
  'es-ES':['Actualizado','No se pudo comprobar. Inténtalo de nuevo.'],
  'id-ID':['Sudah terbaru','Gagal memeriksa pembaruan. Coba lagi.'],
  'uk-UA':['Оновлено','Не вдалося перевірити оновлення. Спробуйте знову.'],
  'tr-TR':['Güncel','Güncellemeler denetlenemedi. Yeniden deneyin.'],
  'pt-BR':['Atualizado','Não foi possível verificar. Tente novamente.'],
  'ar-EG':['محدّث','تعذر فحص التحديثات. حاول مجددًا.'],
  'fa-IR':['به‌روز است','بررسی به‌روزرسانی ناموفق بود. دوباره تلاش کنید.']
 };
 const updateLabels={'zh-CN':'更新版本','zh-TW':'更新版本','en-US':'Update version','ja-JP':'バージョンを更新','ru-RU':'Обновить версию','vi-VN':'Cập nhật phiên bản','es-ES':'Actualizar versión','id-ID':'Perbarui versi','uk-UA':'Оновити версію','tr-TR':'Sürümü güncelle','pt-BR':'Atualizar versão','ar-EG':'تحديث الإصدار','fa-IR':'به‌روزرسانی نسخه'};
 keys.push('upToDate','checkFailed','updateVersion');
 Object.keys(rows).forEach(lang=>rows[lang].push(...inlineRows[lang],updateLabels[lang]));
 const maps=Object.fromEntries(Object.entries(rows).map(([k,v])=>[k,Object.fromEntries(keys.map((key,i)=>[key,v[i]]))]));
 return {keys,maps,get:lang=>maps[lang]||maps['en-US']};
})();
const DuiUpdates = {
 delimiters:['[[',']]'],
 data(){return {expanded:false,state:{items:{panel:{},core:{}}},starting:false,startingKind:'',offline:false,requestError:'',lang:LanguageManager.getLanguage(),timer:null,stopped:false,polling:false,refreshRequested:false}},
 computed:{
  t(){return DuiUpdateText.get(this.lang)},
  job(){return this.state.job||{}},
  percent(){return this.job.total ? Math.max(0,Math.min(100,Math.floor((this.job.downloaded||0)/this.job.total*100))):0},
  offers(){return ['panel','core'].filter(k=>this.state.items[k]?.available)},
  phase(){return this.t[this.job.phase]||this.job.phase||''},
  checking(){return this.state.checking||(!this.job.busy&&this.polling)},
  detailsVisible(){return this.expanded||!!this.job.id||this.starting||this.offline||!!this.state.checkError}
 },
 methods:{
  size(n){if(!Number.isFinite(n))return '—';return (n/1048576).toFixed(1)+' MB'},
  // The legacy core selector reveals this same inline status, never a second dialog.
  show(){this.expanded=true;this.poll();this.$nextTick(()=>this.$el.scrollIntoView({behavior:'smooth',block:'nearest'}))},
  check(){this.expanded=true;return this.poll(true)},
  async poll(force=false){
   if(this.stopped)return;
   if(this.polling){if(force)this.refreshRequested=true;return}
   this.polling=true;
   try{
    const r=force?await axios.post('/dui/api/server/updates/check',{}, {timeout:10000}):await axios.get('/dui/api/server/updates',{timeout:10000});
    if(!r.data?.success||!r.data.obj?.items)throw Error('unavailable');
    if(!this.stopped){this.state=r.data.obj;this.offline=false}
   }catch(e){if(!this.stopped)this.offline=true}
   finally{
    this.polling=false;
    if(this.refreshRequested&&!this.stopped){this.refreshRequested=false;await this.poll(true)}
    else this.schedulePoll()
   }
  },
  schedulePoll(){
   clearTimeout(this.timer);
   if(!this.stopped)this.timer=setTimeout(()=>this.poll(),this.job.busy||this.starting||this.state.checking||this.offline?2000:60000)
  },
  async start(kind){
   if(this.starting||this.job.busy||!this.state.supported||!this.state.items[kind]?.available)return;
   this.expanded=true;this.starting=true;this.startingKind=kind;this.requestError='';
   try{
    const r=await axios.post('/dui/api/server/updates/start',{kind,version:this.state.items[kind].latest},{timeout:15000});
    if(!r.data?.success)throw Error(r.data?.msg||this.t.failed);
    this.$set(this.state,'job',r.data.obj);
   }catch(e){this.requestError=e.message}
   finally{this.starting=false;await this.poll()}
  },
  error(code){
   code=String(code||'').replace(/^\s*\(([^)]+)\)\s*$/,'$1');
   const zh=this.lang==='zh-CN'||this.lang==='zh-TW';
   const reasons={network_error:'无法连接下载服务器',download_interrupted:'下载连接中断，当前版本未替换',size_mismatch:'下载长度不完整或不符，已停止更新',checksum_mismatch:'SHA-256 校验不符，已停止更新',invalid_checksum:'校验文件无效',config_rejected:'新核心未通过当前配置校验',disk_write_failed:'磁盘写入失败，请检查可用空间',health_check_failed:'新版本未通过启动检查',rollback_failed:'自动恢复未完成，请检查 x-ui 服务并使用备份恢复',backup_failed:'备份失败，未替换程序',unsupported_installation:'此安装方式不支持网页更新',update_busy:'已有更新任务正在执行',already_current:'当前已是该版本或更新版本',binary_unusable:'新程序无法在此机器运行',invalid_archive:'压缩包不完整或格式异常',version_mismatch:'程序版本与发布版本不符',recovery_required:'上次更新中断，请先恢复服务',installed_version_changed:'下载期间安装版本已改变，请重新检查更新'};
   return zh?(reasons[code]||code):code;
  },
  reload(){window.location.reload()}
 },
 mounted(){this.poll()},
 beforeDestroy(){this.stopped=true;clearTimeout(this.timer)},
 template:`
 <section class="dui-updates">
  <div class="dui-product-heading">
   <div class="dui-product-title"><slot name="title">[[t.title]]</slot></div>
   <button type="button" class="dui-update-check" :class="{'has-update':offers.length>0}" :disabled="checking||starting||job.busy" :aria-busy="checking" @click.stop="offers.length?show():check()">
    <a-icon :type="checking?'loading':offers.length?'arrow-up':'sync'"/>
    <span v-if="offers.length">[[t.updateVersion]]</span>
    <span v-else>[[checking?t.checking:t.check]]</span>
   </button>
  </div>
  <div v-if="detailsVisible" class="dui-update-inline">
   <div v-for="kind in ['panel','core']" :key="kind" class="dui-update-entry" :data-kind="kind">
    <div class="dui-update-entry-info">
     <div class="dui-update-entry-title"><strong>[[t[kind] ]]</strong><span class="dui-update-current" :title="t.current">[[state.items[kind].current||'—']]</span></div>
     <div class="dui-update-latest"><span>[[t.latest]]</span> <b>[[state.items[kind].latest||t.unknown]]</b></div>
    </div>
    <div class="dui-update-entry-action">
     <span v-if="state.items[kind].available" class="dui-update-available">[[t.available]]</span>
     <button v-if="state.items[kind].available&&state.supported" type="button" class="dui-update-install" :disabled="starting||job.busy" :aria-label="t[kind]+': '+t.start" @click="start(kind)">
      <a-icon :type="starting&&startingKind===kind?'loading':'download'"/><span>[[t.start]]</span>
     </button>
     <span v-else-if="!checking&&!offline&&!state.checkError&&state.items[kind].latest&&!state.items[kind].available" class="dui-update-current-state" role="status"><a-icon type="check-circle"/> [[t.upToDate]]</span>
    </div>
   </div>
   <p v-if="offers.length||job.busy||starting" class="dui-update-hint">[[t.hint]]</p>
   <a-alert v-if="state.supported===false" type="info" show-icon :message="t.unsupported"/>
   <a-alert v-if="offline" type="warning" show-icon :message="job.busy?t.reconnecting:t.checkFailed"/>
   <div v-if="job.id||starting" class="dui-update-progress" aria-live="polite" role="status">
    <div class="dui-update-progress-head"><span>[[t[starting?startingKind:job.kind ] ]] [[starting?state.items[startingKind].latest:job.version]]</span><strong>[[starting?t.connecting:phase]]</strong></div>
    <a-progress v-if="job.total&&(job.busy||job.phase==='complete')&&!starting" :percent="percent" :status="job.phase==='complete'?'success':'active'" :show-info="job.phase==='downloading'"/>
    <div v-else-if="starting||job.busy" class="dui-update-indeterminate" role="progressbar" :aria-label="starting?t.connecting:phase"></div>
    <div v-if="!starting&&(job.total||job.downloaded)" class="dui-update-byte-count">[[size(job.downloaded||0)]]<template v-if="job.total"> / [[size(job.total)]] <span v-if="job.phase==='downloading'">· [[percent]]%</span></template></div>
    <a-alert v-if="job.error&&!starting" type="error" show-icon :message="error(job.error)"/>
    <button v-if="job.phase==='complete'&&job.kind==='panel'" type="button" class="dui-update-install dui-update-reload" @click="reload"><a-icon type="reload"/>[[t.reload]]</button>
   </div>
   <a-alert v-if="requestError||state.checkError" type="warning" show-icon :message="error(requestError||state.checkError)"/>
  </div>
  <div class="dui-product-links"><slot></slot></div>
 </section>`
};
if(typeof Vue!=='undefined')Vue.component('dui-updates',DuiUpdates);
if(typeof module!=='undefined')module.exports={DuiUpdates,DuiUpdateText};
