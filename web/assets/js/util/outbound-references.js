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
        const tags = [...new Set(list(Array.isArray(tag) ? tag : [tag]).filter(value => typeof value === 'string' && value))];
        if (!tags.length) return result;
        visit(config || {}, (obj, key, kind, index) => {
            if (!tags.includes(obj[key])) return;
            if (kind === 'routes') result.routes.push(index);
            else result[kind]++;
            result.total++;
        }, (obj, key, kind) => {
            if (tags.some(value => matches(obj[key], value))) { result[kind]++; result.total++; }
        });
        return result;
    }

    // Draft-only history follows the tag, not a row index, and never enters the Xray payload.
    function pruneRenames(history, config) {
        const tags = new Set(list(config?.outbounds).map(item => item?.tag));
        return list(history).filter(entry => entry && tags.has(entry.tag)).map(entry => ({
            tag: entry.tag,
            previousTags: [...new Set(list(entry.previousTags))].filter(tag =>
                typeof tag === 'string' && tag && !tags.has(tag) && inspect(config, tag).total > 0)
        })).filter(entry => entry.previousTags.length > 0);
    }

    function pendingTags(history, config, index) {
        const tag = config?.outbounds?.[index]?.tag;
        return pruneRenames(history, config).find(entry => entry.tag === tag)?.previousTags || [];
    }

    function rememberRename(history, config, index, next, mode) {
        const oldTag = config.outbounds[index].tag;
        const newTag = next.outbounds[index].tag;
        const clean = pruneRenames(history, config);
        const previousTags = pendingTags(clean, config, index);
        const result = clean.filter(entry => entry.tag !== oldTag);
        if (mode === 'replace') {
            result.push({ tag: newTag, previousTags: [...previousTags, ...(oldTag !== newTag ? [oldTag] : [])] });
        }
        return pruneRenames(result, next);
    }

    function prepare(config, index, outbound, mode = 'sync', previousTags = []) {
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
        if (mode === 'replace') {
            next.outbounds[index] = edited;
            return next;
        }
        const oldTags = config.outbounds.map(item => item?.tag).filter(tag => typeof tag === 'string');
        // Do not claim an old name now owned by another outbound.
        const sourceTags = [oldTag, ...list(previousTags).filter(tag =>
            typeof tag === 'string' && tag && !oldTags.includes(tag))];
        next.outbounds[index] = edited;
        visit(next, (obj, key) => {
            if (sourceTags.includes(obj[key])) obj[key] = newTag;
        }, (obj, key) => {
            if (!Array.isArray(obj[key])) return;
            const before = obj[key].slice();
            const wasSelected = sourceTags.some(tag => matches(before, tag));
            let after = before.slice();
            if (wasSelected) {
                // A full tag can also be a prefix for other nodes. Retain that prefix when shared.
                after = after.map(prefix => sourceTags.includes(prefix) &&
                    !oldTags.some(tag => tag !== oldTag && tag.startsWith(prefix)) ? newTag : prefix);
                if (!matches(after, newTag)) after.push(newTag);
            }
            // Preserve membership of every existing node; broad prefixes must not silently expand a group.
            for (const tag of oldTags) {
                const renamed = tag === oldTag ? newTag : tag;
                const selectedBefore = tag === oldTag ? wasSelected : matches(before, tag);
                if (selectedBefore !== matches(after, renamed)) {
                    throw new Error('新标签会改变负载均衡或观测分组的前缀匹配范围。请换一个标签，或先手动调整分组。');
                }
            }
            if (JSON.stringify(before) !== JSON.stringify(after)) obj[key] = [...new Set(after)];
        });
        return next;
    }

    return { inspect, prepare, pruneRenames, pendingTags, rememberRename };
})();
