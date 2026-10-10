// UI labels are localized; core field names and outbound tags remain unchanged.
const DuiStrategyProbes = (() => {
    const types = ['leastPing', 'leastLoad', 'random', 'roundRobin'];
    const clone = value => JSON.parse(JSON.stringify(value));
    const plain = value => value && typeof value === 'object' && !Array.isArray(value);
    function selectors(config, type) {
        return [...new Set((config.routing?.balancers || []).filter(b => (b.strategy?.type || 'random') === type)
            .flatMap(b => b.selector || []))];
    }
    function read(config, type) {
        const fallback = type === 'leastPing'
            ? { subjectSelector: [], probeURL: 'http://www.google.com/gen_204', probeInterval: '30s', enableConcurrency: true }
            : { subjectSelector: [], pingConfig: { destination: 'http://www.google.com/gen_204', interval: '30s',
                connectivity: 'http://connectivitycheck.platform.hicloud.com/generate_204', timeout: '10s', sampling: 2 } };
        const explicit = config.strategyObservatory?.[type];
        const legacy = config[type === 'leastPing' ? 'observatory' : 'burstObservatory'];
        const result = clone(explicit || legacy || fallback);
        // Fill omitted core defaults, not the panel's first-install defaults.
        if (type === 'leastPing') {
            if (result.probeURL == null) result.probeURL = 'https://www.google.com/generate_204';
            if (result.probeInterval == null) result.probeInterval = '10s';
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
        next.strategyObservatory = next.strategyObservatory || {};
        next.strategyObservatory[type] = clone(value);
        next.strategyObservatory[type].subjectSelector = selectors(config, type);
        return next;
    }
    function syncSelectors(config) {
        for (const type of types) {
            if (config.strategyObservatory?.[type]) config.strategyObservatory[type].subjectSelector = selectors(config, type);
        }
        return config;
    }
    return {types, clone, read, update, selectors, syncSelectors, validate, duration};
})();
if (typeof module !== 'undefined') module.exports = DuiStrategyProbes;
