// Merge only the visible page/filter slots; hidden rows retain their positions.
const DuiInboundOrder = {
    merge(all, before, after) {
        if (!Array.isArray(before) || before.length !== after?.length || !before.length) return null;
        const selected = new Set(before);
        if (selected.size !== before.length || new Set(after).size !== before.length ||
            after.some(id => !selected.has(id))) return null;
        const current = all.filter(id => selected.has(id));
        if (JSON.stringify(current) !== JSON.stringify(before)) return null;
        let i = 0;
        return all.map(id => selected.has(id) ? after[i++] : id);
    },
};
if (typeof module !== 'undefined') module.exports = DuiInboundOrder;

if (typeof Vue !== 'undefined') Vue.component('dui-inbound-sort-handle', {
    props: { items: Array, disabled: Boolean, label: String, inboundId: Number },
    template: '<button type="button" class="dui-inbound-sort-handle" :data-inbound-id="inboundId" :title="label" :aria-label="label" :disabled="disabled || items.length < 2" @click.stop.prevent @pointerdown="start" @keydown="key" @lostpointercapture="cancel"><a-icon type="drag"></a-icon></button>',
    watch: { items() { this.cancel(); }, disabled(value) { if (value) this.cancel(); } },
    beforeDestroy() { this.cancel(); },
    methods: {
        rows() {
            return [...(this.$el.closest('tbody')?.children || [])].filter(row =>
                row.classList.contains('ant-table-row') && row.hasAttribute('data-row-key'));
        },
        emitMove(ids, from, to) {
            const after = ids.slice();
            after.splice(to, 0, after.splice(from, 1)[0]);
            this.$emit('reorder', {before: ids, after, focusId: this.inboundId});
        },
        key(event) {
            if (event.key === 'Escape') { this.cancel(); return; }
            if (this.disabled) return;
            const offset = event.key === 'ArrowUp' ? -1 : event.key === 'ArrowDown' ? 1 : 0;
            if (!offset) return;
            event.preventDefault();
            this.cancel();
            const ids = this.rows().map(row => Number(row.dataset.rowKey));
            const from = ids.indexOf(this.inboundId), to = from + offset;
            if (from >= 0 && to >= 0 && to < ids.length) this.emitMove(ids, from, to);
        },
        start(event) {
            if (this.disabled || event.button !== 0 || event.isPrimary === false) return;
            this.cancel();
            const body = this.$el.closest('tbody'), rows = this.rows();
            const ids = rows.map(row => Number(row.dataset.rowKey)), from = ids.indexOf(this.inboundId);
            if (!body || ids.length < 2 || from < 0) return;
            event.preventDefault();
            this.$el.focus({preventScroll: true});
            const scrollParents = [];
            for (let el = body.parentElement; el; el = el.parentElement) {
                if (/(auto|scroll)/.test(getComputedStyle(el).overflowY) && el.scrollHeight > el.clientHeight) scrollParents.push(el);
            }
            if (!scrollParents.includes(document.scrollingElement)) scrollParents.push(document.scrollingElement);
            this._drag = {id: event.pointerId, from, to: from, ids, body,
                x: event.clientX, y: event.clientY, startX: event.clientX, startY: event.clientY,
                moved: false, valid: false, scrollParents, mark: null};
            this.$emit('dragging', true);
            this.$el.setPointerCapture?.(event.pointerId);
            document.addEventListener('pointermove', this.move, {passive: false});
            document.addEventListener('pointerup', this.finish);
            document.addEventListener('pointercancel', this.cancel);
            document.addEventListener('keydown', this.escape);
            window.addEventListener('blur', this.cancel);
            this._frame = requestAnimationFrame(this.tick);
        },
        move(event) {
            const drag = this._drag;
            if (!drag || event.pointerId !== drag.id) return;
            event.preventDefault();
            drag.x = event.clientX; drag.y = event.clientY;
            drag.moved ||= Math.hypot(drag.x - drag.startX, drag.y - drag.startY) >= 5;
            this.locate();
        },
        locate() {
            const drag = this._drag;
            if (!drag || !drag.moved) return;
            const rows = this.rows(), rect = drag.body.getBoundingClientRect();
            drag.valid = JSON.stringify(rows.map(row => Number(row.dataset.rowKey))) === JSON.stringify(drag.ids) &&
                drag.x >= rect.left && drag.x <= rect.right &&
                drag.y >= Math.max(0, rect.top - 24) && drag.y <= Math.min(innerHeight, rect.bottom + 24);
            if (drag.mark) drag.mark.classList.remove('dui-inbound-drop-before', 'dui-inbound-drop-after');
            drag.mark = null;
            rows[drag.from]?.classList.add('dui-inbound-drag-source');
            if (!drag.valid) return;
            // An expanded client section belongs to its parent inbound.
            let target = -1;
            rows.forEach((row, index) => { if (drag.y >= row.getBoundingClientRect().top) target = index; });
            if (target < 0) target = 0;
            drag.to = target;
            if (target === drag.from) return;
            drag.mark = rows[target];
            drag.mark.classList.add(target < drag.from ? 'dui-inbound-drop-before' : 'dui-inbound-drop-after');
        },
        tick() {
            const drag = this._drag;
            if (!drag) return;
            if (drag.moved) {
                const table = drag.body.getBoundingClientRect();
                if (drag.x >= table.left && drag.x <= table.right) {
                    for (const parent of drag.scrollParents) {
                        const rect = parent === document.scrollingElement ? {top: 0, bottom: innerHeight} : parent.getBoundingClientRect();
                        const top = Math.max(0, rect.top), bottom = Math.min(innerHeight, rect.bottom);
                        const direction = drag.y < top + 44 ? -1 : drag.y > bottom - 44 ? 1 : 0;
                        if (!direction) continue;
                        const before = parent.scrollTop;
                        parent.scrollTop += direction * 10;
                        if (before !== parent.scrollTop) break;
                    }
                }
                this.locate();
            }
            this._frame = requestAnimationFrame(this.tick);
        },
        finish(event) {
            const drag = this._drag;
            if (!drag || event.pointerId !== drag.id) return;
            drag.x = event.clientX; drag.y = event.clientY;
            this.locate();
            const apply = drag.moved && drag.valid && drag.to !== drag.from;
            this.cancel();
            if (apply) this.emitMove(drag.ids, drag.from, drag.to);
        },
        escape(event) { if (event.key === 'Escape') this.cancel(); },
        cancel() {
            if (this._frame) cancelAnimationFrame(this._frame);
            this._frame = null;
            const drag = this._drag; this._drag = null;
            if (drag) {
                drag.body.querySelectorAll('tr').forEach(row => row.classList.remove(
                    'dui-inbound-drag-source', 'dui-inbound-drop-before', 'dui-inbound-drop-after'));
                if (this.$el.hasPointerCapture?.(drag.id)) this.$el.releasePointerCapture(drag.id);
                this.$emit('dragging', false);
            }
            document.removeEventListener('pointermove', this.move);
            document.removeEventListener('pointerup', this.finish);
            document.removeEventListener('pointercancel', this.cancel);
            document.removeEventListener('keydown', this.escape);
            window.removeEventListener('blur', this.cancel);
        },
    },
});
