const DuiSpeedTestI18n = (() => {
 const en={
 title:'Download speed test',outbounds:'Select outbounds',mode:'Connections',single:'Single connection',multi:'Multiple connections',threads:'Parallel connections',
 start:'Start test',stop:'Stop',close:'Collapse',refresh:'Refresh',average:'Average',current:'Live speed',traffic:'Downloaded',elapsed:'Duration',
 hint:'Runs on the panel server through each selected outbound to Cloudflare, not in your browser. Tests run one outbound at a time: 10 seconds of downloading, up to 512 MiB each. This uses bandwidth and may affect active traffic.',
 saved:'Uses saved settings. Save your outbound changes before testing.',empty:'Select 1–10 outbounds. Non-download and interface-based outbounds are excluded.',
 results:'Latest test',queued:'Queued',preparing:'Preparing',connecting:'Connecting',testing:'Testing',done:'Completed',partial:'Partially completed',failed:'Failed',cancelled:'Stopped',running:'Running',stopping:'Stopping',idle:'Ready',
 limit:'Traffic limit reached',error:'Request failed. Refresh to retrieve the current task.',busy:'Another test is running. Refresh to view it.',
 download_failed:'No download data received; check the outbound or try again later.',stream_failed:'Some download connections failed; the result is incomplete.',
 worker_start:'The test core could not start. Check that this outbound is supported by the installed core.',
 unsupported_outbound:'This outbound or its proxy chain is not supported for an isolated download test.',missing_outbound:'The saved outbound no longer exists. Refresh the list.',
 invalid_request:'Select 1–10 outbounds and 1–16 parallel connections.',core_unavailable:'The installed core is unavailable.',stale_job:'The task has changed. Refresh its status.',
 config_unavailable:'Saved configuration could not be read.',invalid_config:'The saved configuration is invalid.',invalid_chain:'The proxy chain contains a cycle.',duplicate_tag:'Saved outbound tags are duplicated.'
 };
 const zh={
 title:'下载测速',outbounds:'选择出口',mode:'测速模式',single:'单线程',multi:'多线程',threads:'并发数',
 start:'开始测速',stop:'停止测速',close:'收起',refresh:'刷新',average:'平均速度',current:'实时速度',traffic:'已下载',elapsed:'用时',
 hint:'由面板机器经过所选出口连接 Cloudflare 测速站，不使用浏览器本地网络。多个出口依次测试，每个出口下载 10 秒、最多 512 MiB；会消耗流量并可能影响当前业务带宽。',
 saved:'使用已保存的出口设置。修改出口后，请先保存再测速。',empty:'可选 1–10 个出口。不支持下载或需要创建网络接口的出口不会列出。',
 results:'最近一次测速',queued:'等待中',preparing:'准备中',connecting:'连接中',testing:'测速中',done:'已完成',partial:'部分完成',failed:'失败',cancelled:'已停止',running:'正在测速',stopping:'正在停止',idle:'就绪',
 limit:'已达到流量上限',error:'请求失败，请刷新获取当前任务状态。',busy:'已有测速任务在运行，请刷新查看。',
 download_failed:'未收到下载数据，请检查出口或稍后重试。',stream_failed:'部分下载连接失败，本次结果不完整。',
 worker_start:'测速内核启动失败，请检查已安装内核是否支持该出口配置。',
 unsupported_outbound:'该出口或其代理链暂不支持独立下载测速。',missing_outbound:'已保存的出口不存在，请刷新列表。',
 invalid_request:'请选择 1–10 个出口，并发数为 1–16。',core_unavailable:'未找到可用的已安装内核。',stale_job:'任务已变化，请刷新状态。',
 config_unavailable:'无法读取已保存的配置。',invalid_config:'已保存的配置无效。',invalid_chain:'出口代理链存在循环引用。',duplicate_tag:'已保存的出口标签重复。'
 };
 const tw={...zh,title:'下載測速',outbounds:'選擇出口',mode:'測速模式',single:'單執行緒',multi:'多執行緒',threads:'並行數',start:'開始測速',stop:'停止測速',close:'收起',refresh:'重新整理',average:'平均速度',current:'即時速度',traffic:'已下載',elapsed:'用時',
 hint:'由面板主機經過所選出口連接 Cloudflare 測速站，不使用瀏覽器本機網路。多個出口依次測試，每個出口下載 10 秒、最多 512 MiB；會消耗流量並可能影響目前業務頻寬。',
 saved:'使用已儲存的出口設定。修改出口後，請先儲存再測速。',empty:'可選 1–10 個出口。不支援下載或需要建立網路介面的出口不會列出。',results:'最近一次測速',queued:'等候中',preparing:'準備中',connecting:'連接中',testing:'測速中',done:'已完成',partial:'部分完成',failed:'失敗',cancelled:'已停止',running:'正在測速',stopping:'正在停止',idle:'就緒',limit:'已達到流量上限',error:'請求失敗，請重新整理取得目前任務狀態。'};
 const keys=['title','outbounds','mode','single','multi','threads','start','stop','close','refresh','average','current','traffic','elapsed','results','queued','preparing','connecting','testing','done','partial','failed','cancelled','running','stopping','idle'];
 const rows={
 'ja-JP':['ダウンロード速度テスト','出口を選択','接続モード','単一接続','複数接続','並列接続数','開始','停止','折りたたむ','更新','平均速度','現在の速度','ダウンロード量','時間','最新のテスト','待機中','準備中','接続中','測定中','完了','一部完了','失敗','停止済み','実行中','停止中','準備完了'],
 'ru-RU':['Тест скорости загрузки','Выходы','Режим','Одно соединение','Несколько соединений','Число соединений','Начать','Остановить','Свернуть','Обновить','Средняя скорость','Текущая скорость','Загружено','Время','Последний тест','В очереди','Подготовка','Соединение','Измерение','Завершено','Частично','Ошибка','Остановлено','Выполняется','Остановка','Готово'],
 'vi-VN':['Đo tốc độ tải xuống','Chọn đầu ra','Chế độ','Một kết nối','Nhiều kết nối','Số kết nối','Bắt đầu','Dừng','Thu gọn','Làm mới','Trung bình','Hiện tại','Đã tải','Thời gian','Lần đo gần nhất','Đang chờ','Chuẩn bị','Kết nối','Đang đo','Hoàn tất','Một phần','Thất bại','Đã dừng','Đang chạy','Đang dừng','Sẵn sàng'],
 'fa-IR':['آزمون سرعت دانلود','انتخاب خروجی','حالت اتصال','تک اتصال','چند اتصال','تعداد اتصال','شروع','توقف','بستن','تازه‌سازی','میانگین','سرعت فعلی','دانلودشده','زمان','آخرین آزمون','در صف','آماده‌سازی','اتصال','در حال سنجش','کامل','ناقص','ناموفق','متوقف','در حال اجرا','در حال توقف','آماده'],
 'ar-SA':['اختبار سرعة التنزيل','اختيار المخارج','وضع الاتصال','اتصال واحد','اتصالات متعددة','عدد الاتصالات','بدء','إيقاف','طي','تحديث','المتوسط','السرعة الحالية','تم تنزيله','المدة','آخر اختبار','في الانتظار','تحضير','اتصال','اختبار','مكتمل','مكتمل جزئيا','فشل','متوقف','قيد التشغيل','جار الإيقاف','جاهز'],
 'es-ES':['Prueba de descarga','Seleccionar salidas','Conexiones','Una conexión','Varias conexiones','Conexiones paralelas','Iniciar','Detener','Contraer','Actualizar','Media','Velocidad actual','Descargado','Duración','Última prueba','En cola','Preparando','Conectando','Probando','Completado','Parcial','Error','Detenido','En ejecución','Deteniendo','Listo'],
 'de-DE':['Downloadtest','Ausgänge wählen','Verbindungen','Einzelverbindung','Mehrere Verbindungen','Parallele Verbindungen','Starten','Stoppen','Einklappen','Aktualisieren','Durchschnitt','Aktuelle Geschwindigkeit','Heruntergeladen','Dauer','Letzter Test','Wartend','Vorbereitung','Verbinden','Test läuft','Abgeschlossen','Teilweise','Fehlgeschlagen','Gestoppt','Läuft','Wird gestoppt','Bereit'],
 'fr-FR':['Test de téléchargement','Choisir les sorties','Connexions','Connexion unique','Connexions multiples','Connexions parallèles','Démarrer','Arrêter','Réduire','Actualiser','Moyenne','Vitesse actuelle','Téléchargé','Durée','Dernier test','En attente','Préparation','Connexion','Test en cours','Terminé','Partiel','Échec','Arrêté','En cours','Arrêt en cours','Prêt'],
 'id-ID':['Uji unduhan','Pilih keluaran','Koneksi','Satu koneksi','Banyak koneksi','Koneksi paralel','Mulai','Hentikan','Ciutkan','Segarkan','Rata-rata','Kecepatan saat ini','Diunduh','Durasi','Uji terakhir','Antrean','Persiapan','Menghubungkan','Menguji','Selesai','Sebagian','Gagal','Dihentikan','Berjalan','Menghentikan','Siap'],
 'tr-TR':['İndirme hız testi','Çıkışları seç','Bağlantılar','Tek bağlantı','Çoklu bağlantı','Paralel bağlantı','Başlat','Durdur','Daralt','Yenile','Ortalama','Anlık hız','İndirilen','Süre','Son test','Kuyrukta','Hazırlanıyor','Bağlanıyor','Test ediliyor','Tamamlandı','Kısmen','Başarısız','Durduruldu','Çalışıyor','Durduruluyor','Hazır'],
 'uk-UA':['Тест завантаження','Виберіть виходи','З’єднання','Одне з’єднання','Кілька з’єднань','Паралельні з’єднання','Почати','Зупинити','Згорнути','Оновити','Середня','Поточна швидкість','Завантажено','Час','Останній тест','У черзі','Підготовка','З’єднання','Тестування','Завершено','Частково','Помилка','Зупинено','Виконується','Зупинка','Готово']
 };
 rows['pt-BR']=['Teste de download','Selecionar saídas','Conexões','Conexão única','Múltiplas conexões','Conexões paralelas','Iniciar','Parar','Recolher','Atualizar','Média','Velocidade atual','Baixado','Duração','Último teste','Na fila','Preparando','Conectando','Testando','Concluído','Parcial','Falhou','Interrompido','Em execução','Parando','Pronto'];
 rows['ar-EG']=rows['ar-SA'];
 const maps={'en-US':en,'zh-CN':{...en,...zh},'zh-TW':{...en,...tw}};
 for(const [lang,row]of Object.entries(rows))maps[lang]={...en,...Object.fromEntries(keys.map((key,i)=>[key,row[i]]))};
 return {get:lang=>maps[lang]||en,maps};
})();
if(typeof module!=='undefined')module.exports=DuiSpeedTestI18n;
