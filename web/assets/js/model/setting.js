class AllSetting {

    constructor(data) {
        this.webListen = "";
        this.webDomain = "";
        this.webPort = 13688;
        this.webCertFile = "";
        this.webKeyFile = "";
        this.webBasePath = "/shlii/";
        this.sessionMaxAge = 360;
        this.pageSize = 50;
        this.expireDiff = 0;
        this.trafficDiff = 0;
        this.remarkModel = "-ieo";
        this.datepicker = "gregorian";
        this.tgBotEnable = false;
        this.tgBotToken = "";
        this.tgBotProxy = "";
        this.tgBotAPIServer = "";
        this.tgBotChatId = "";
        this.tgRunTime = "@daily";
        this.tgBotBackup = false;
        this.tgBotLoginNotify = true;
        this.tgPanelName = "";
        this.tgNotifyLoginSuccess = true;
        this.tgNotifyLoginFail = true;
        this.tgNotifyPanelBruteForce = true;
        this.tgNotifySSHBruteForce = true;
        this.tgNotifyTimeSync = true;
        this.tgNotifyDDNS = true;
        this.tgNotifyCPU = false;
        this.tgCPUThreshold = 90;
        this.tgCPUDuration = 300;
        this.tgNotifyMemory = false;
        this.tgMemoryThreshold = 90;
        this.tgMemoryDuration = 300;
        this.tgNotifyDisk = false;
        this.tgDiskThreshold = 90;
        this.tgCpu = 80;
        this.tgLang = "zh-CN";
        this.twoFactorEnable = false;
        this.twoFactorToken = "";
        this.xrayTemplateConfig = "";
        this.subEnable = false;
        this.subTitle = "";
        this.subListen = "";
        this.subPort = 13788;
        this.subPath = "/sub/";
        this.subJsonPath = "/json/";
        this.subDomain = "";
        this.externalTrafficInformEnable = false;
        this.externalTrafficInformURI = "";
        this.subCertFile = "";
        this.subKeyFile = "";
        this.subUpdates = 12;
        this.subEncrypt = true;
        this.subShowInfo = true;
        this.subURI = "";
        this.subJsonURI = "";
        this.subJsonFragment = "";
        this.subJsonNoises = "";
        this.subJsonMux = "";
        this.subJsonRules = "";

        this.timeLocation = "Local";

        if (data == null) {
            return
        }
        ObjectUtil.cloneProps(this, data);
    }

    equals(other) {
        return ObjectUtil.equals(this, other);
    }
}
