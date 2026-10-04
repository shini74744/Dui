// Only visit fields that refer to outbound tags, never arbitrary strings in node settings.
const DuiOutboundReferences = (() => {
    const clone = value => JSON.parse(JSON.stringify(value));
    const list = value => Array.isArray(value) ? value : [];
    const matches = (selectors, tag) => list(selectors).some(prefix => typeof prefix === 'string' && tag.startsWith(prefix));

    function visit(config, exact, selector) {
        list(config.routing?.rules).forEach((rule, index) => {
            if (rule) exact(rule, 'outboundTag', 'routes', index + 1);
        });
        list(config.routing?.balancers).forEach(balancer => {
            if (!balancer) return;
            exact(balancer, 'fallbackTag', 'balancers');
            selector(balancer, 'selector', 'balancers');
        });
        for (const key of ['observatory', 'burstObservatory']) {
            if (config[key]) selector(config[key], 'subjectSelector', 'observatories');
        }
        function streamRefs(stream) {
            if (!stream) return;
            if (stream.sockopt) exact(stream.sockopt, 'dialerProxy', 'chains');
            const download = stream.xhttpSettings?.extra?.downloadSettings;
            if (download?.sockopt) exact(download.sockopt, 'dialerProxy', 'chains');
        }
        for (const outbound of list(config.outbounds)) {
            if (!outbound) continue;
            if (outbound.proxySettings) exact(outbound.proxySettings, 'tag', 'chains');
            streamRefs(outbound.streamSettings);
        }
        list(config.inbounds).forEach(inbound => streamRefs(inbound?.streamSettings));
    }

    function inspect(config, tag) {
        const result = { routes: [], balancers: 0, observatories: 0, chains: 0, total: 0 };
        if (!tag) return result;
        visit(config || {}, (obj, key, kind, index) => {
            if (obj[key] !== tag) return;
            if (kind === 'routes') result.routes.push(index);
            else result[kind]++;
            result.total++;
        }, (obj, key, kind) => {
            if (matches(obj[key], tag)) { result[kind]++; result.total++; }
        });
        return result;
    }

    function prepare(config, index, outbound, mode = 'sync') {
        const original = config?.outbounds?.[index];
        if (!original) throw new Error('原出站已不存在，请关闭窗口后重新编辑。');
        const newTag = String(outbound?.tag || '').trim();
        const oldTag = original.tag;
        if (!newTag) throw new Error('请填写出站标签。');
        if (newTag.startsWith('__dui_blacklist_')) throw new Error('该标签前缀由系统使用，请换一个标签。');
        if (config.outbounds.some((item, i) => i !== index && String(item?.tag || '').trim() === newTag)) {
            throw new Error('出站标签已存在，请使用其他标签。');
        }
        if (!['sync', 'replace'].includes(mode)) throw new Error('请选择同步保存或普通确定。');
        const next = clone(config);
        const edited = clone(outbound);
        edited.tag = newTag;
        if (oldTag === newTag) {
            next.outbounds[index] = edited;
            return next;
        }
        if (mode === 'replace') {
            next.outbounds[index] = edited;
            return next;
        }
        const oldTags = config.outbounds.map(item => item?.tag).filter(tag => typeof tag === 'string');
        next.outbounds[index] = edited;
        visit(next, (obj, key) => {
            if (obj[key] === oldTag) obj[key] = newTag;
        }, (obj, key) => {
            if (!Array.isArray(obj[key])) return;
            const before = obj[key].slice();
            const wasSelected = matches(before, oldTag);
            let after = before.slice();
            if (wasSelected) {
                // A full tag can also be a prefix for other nodes. Retain that prefix when shared.
                const sharedPrefix = oldTags.some(tag => tag !== oldTag && tag.startsWith(oldTag));
                after = after.map(prefix => prefix === oldTag && !sharedPrefix ? newTag : prefix);
                if (!matches(after, newTag)) after.push(newTag);
            }
            // Preserve membership of every existing node; broad prefixes must not silently expand a group.
            for (const tag of oldTags) {
                const renamed = tag === oldTag ? newTag : tag;
                if (matches(before, tag) !== matches(after, renamed)) {
                    throw new Error('新标签会改变负载均衡或观测分组的前缀匹配范围。请换一个标签，或先手动调整分组。');
                }
            }
            if (JSON.stringify(before) !== JSON.stringify(after)) obj[key] = [...new Set(after)];
        });
        return next;
    }

    return { inspect, prepare };
})();
