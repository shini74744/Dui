// UI labels are localized; core field names and outbound tags remain unchanged.
const DuiStrategyProbes = (() => {
    const types = ['leastPing', 'leastLoad', 'random', 'roundRobin', 'outbound'];
    const clone = value => JSON.parse(JSON.stringify(value));
    const plain = value => value && typeof value === 'object' && !Array.isArray(value);
    const balancingTypes = types.filter(type => type !== 'outbound');
    const probeable = config => (config.outbounds || []).filter(o => o.tag && o.tag !== 'api' && !o.tag.startsWith('__dui_blacklist_') && !['blackhole','dns','loopback'].includes(o.protocol)).map(o => o.tag);
    function selectors(config, type) {
        if (type === 'outbound') {
            const eligible = new Set(probeable(config));
            return [...new Set((config.routing?.rules || []).filter(r => !r.balancerTag)
                .flatMap(r => [r.outboundTag, r.fallbackTag]).filter(t => eligible.has(t)))];
        }
        return [...new Set((config.routing?.balancers || []).flatMap(b =>
            b.strategy?.type === 'priority' ? (b.strategy.settings?.levels || []) : [b])
            .filter(b => (b.strategy?.type || 'random') === type).flatMap(b => b.selector || []))];
    }
    function enable(config, type) {
        if (!config.strategyObservatory?.[type]) {
            config.strategyObservatory = config.strategyObservatory || {};
            config.strategyObservatory[type] = clone(config.duiDisabledProbes?.[type] || read(config, type));
            if (config.duiDisabledProbes) {
                delete config.duiDisabledProbes[type];
                if (!Object.keys(config.duiDisabledProbes).length) delete config.duiDisabledProbes;
            }
        }
        return syncSelectors(config);
    }
    const enabled = (config, type) => !!config.strategyObservatory?.[type];
    const fallbackRequired = config => (config.routing?.rules || []).some(r => !!r.fallbackTag);
    function setEnabled(config, on) {
        const next = clone(config);
        if (on) {
            if (enabled(next,'outbound')) return next;
            next.strategyObservatory = next.strategyObservatory || {};
            next.strategyObservatory.outbound = clone(next.duiDisabledProbes?.outbound || read(next,'outbound'));
            next.strategyObservatory.outbound.subjectSelector = selectors(next,'outbound');
            if (next.duiDisabledProbes) {
                delete next.duiDisabledProbes.outbound;
                if (!Object.keys(next.duiDisabledProbes).length) delete next.duiDisabledProbes;
            }
            return next;
        }
        if (fallbackRequired(config)) throw new Error('fallback requires probes');
        if (next.strategyObservatory?.outbound) {
            // Panel-only metadata; excluded from the core's runtime Config.
            next.duiDisabledProbes = next.duiDisabledProbes || {};
            next.duiDisabledProbes.outbound = clone(next.strategyObservatory.outbound);
            delete next.strategyObservatory.outbound;
            if (!Object.keys(next.strategyObservatory).length) delete next.strategyObservatory;
        }
        return next;
    }
    function enablePriority(config) {
        for (const b of config.routing?.balancers || []) {
            if (b.strategy?.type === 'priority') for (const l of b.strategy.settings?.levels || [])
                enable(config, l.strategy?.type || 'random');
        }
        return syncSelectors(config);
    }
    function balancerValue(draft) {
        const result = {tag:draft.tag};
        if (draft.strategy === 'priority') {
            const levels = clone(draft.levels || []);
            if (!levels.length || levels.length > 10 || levels.some(l => !l.selector?.length || !balancingTypes.includes(l.strategy?.type))) throw new Error('invalid priority levels');
            result.strategy = {type:'priority', settings:{levels}};
        } else {
            result.selector = clone(draft.selector || []);
            result.fallbackTag = draft.fallbackTag || '';
            result.strategy = {type:draft.strategy || 'random'};
            if (draft.strategySettings) result.strategy.settings = clone(draft.strategySettings);
        }
        return result;
    }
    function read(config, type) {
        const fallback = type === 'leastPing'
            ? { subjectSelector: [], probeURL: 'http://www.google.com/gen_204', probeInterval: '1m', enableConcurrency: true }
            : { subjectSelector: [], pingConfig: { destination: 'http://www.google.com/gen_204', interval: '1m',
                connectivity: 'http://connectivitycheck.platform.hicloud.com/generate_204', timeout: '10s', sampling: 2 } };
        const explicit = config.strategyObservatory?.[type] || config.duiDisabledProbes?.[type];
        const legacy = type === 'outbound' ? null : config[type === 'leastPing' ? 'observatory' : 'burstObservatory'];
        const result = clone(explicit || legacy || fallback);
        // Fill omitted core defaults, not the panel's first-install defaults.
        if (type === 'leastPing') {
            if (result.probeURL == null) result.probeURL = 'https://www.google.com/generate_204';
            if (result.probeInterval == null) result.probeInterval = '1m';
            if (result.enableConcurrency == null) result.enableConcurrency = false;
        } else {
            result.pingConfig = result.pingConfig || {};
            const coreDefaults = {destination:'https://connectivitycheck.gstatic.com/generate_204', interval:'1m', connectivity:'', timeout:'5s', sampling:10};
            for (const [key,value] of Object.entries(coreDefaults)) if (result.pingConfig[key] == null) result.pingConfig[key] = value;
        }
        result.subjectSelector = selectors(config, type);
        return result;
    }
    function duration(value) {
        if (typeof value !== 'string' || !/^(\d+(\.\d*)?|\.\d+)(ns|us|µs|μs|ms|s|m|h)(?:(\d+(\.\d*)?|\.\d+)(ns|us|µs|μs|ms|s|m|h))*$/.test(value)) return NaN;
        const scale = {ns:1e-9,us:1e-6,'µs':1e-6,'μs':1e-6,ms:.001,s:1,m:60,h:3600};
        return [...value.matchAll(/(\d+(?:\.\d*)?|\.\d+)(ns|us|µs|μs|ms|s|m|h)/g)]
            .reduce((total, match) => total + Number(match[1]) * scale[match[2]], 0);
    }
    function url(value, empty = false) {
        if (value === '') return empty;
        try { const parsed = new URL(value); return ['http:', 'https:'].includes(parsed.protocol) && !!parsed.hostname; } catch (_) {return false;}
    }
    function validate(type, value) {
        if (!types.includes(type) || !plain(value)) return 'invalid';
        if (type === 'leastPing') {
            if (!url(value.probeURL, true)) return 'urlError';
            const seconds = duration(value.probeInterval);
            if (!(seconds > 0) || seconds > 9.22e9) return 'intervalError';
            if (typeof value.enableConcurrency !== 'boolean') return 'invalid';
        } else {
            if (!plain(value.pingConfig)) return 'invalid';
            const p = value.pingConfig, seconds = duration(p.interval), timeout = duration(p.timeout);
            if (!url(p.destination, true) || !url(p.connectivity, true)) return 'urlError';
            if (!(seconds >= 10)) return 'burstIntervalError';
            if (!(timeout > 0) || timeout > 9.22e9) return 'timeoutError';
            if (!Number.isInteger(p.sampling) || p.sampling < 1 || p.sampling > 1000 || seconds*p.sampling*2 > 9.22e9) return 'samplingError';
        }
        return '';
    }
    function update(config, type, value) {
        const error = validate(type, value);
        if (error) throw new Error(error);
        const next = clone(config);
        const key = type === 'outbound' && !enabled(config, type) ? 'duiDisabledProbes' : 'strategyObservatory';
        next[key] = next[key] || {};
        next[key][type] = clone(value);
        next[key][type].subjectSelector = selectors(config, type);
        return next;
    }
    function syncSelectors(config) {
        for (const type of types) {
            if (config.strategyObservatory?.[type]) config.strategyObservatory[type].subjectSelector = selectors(config, type);
        }
        return config;
    }
    return {enabled, setEnabled, fallbackRequired, types, balancingTypes, probeable, enable, enablePriority, balancerValue, clone, read, update, selectors, syncSelectors, validate, duration};
})();
if (typeof module !== 'undefined') module.exports = DuiStrategyProbes;
