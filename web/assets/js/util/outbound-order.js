// Reorder visible outbounds while retaining internal blacklist entries and all outbound contents.
const DuiOutboundOrder = (() => {
    function move(outbounds, from, to) {
        if (!Array.isArray(outbounds)) return outbounds;
        const positions = [];
        outbounds.forEach((item, index) => {
            if (!String(item?.tag || '').startsWith('__dui_blacklist_')) positions.push(index);
        });
        if (![from, to].every(index => Number.isInteger(index) && index >= 0 && index < positions.length) || from === to) return outbounds;
        const visible = positions.map(index => outbounds[index]);
        visible.splice(to, 0, visible.splice(from, 1)[0]);
        const result = outbounds.slice();
        positions.forEach((index, i) => { result[index] = visible[i]; });
        // A hidden helper must never become the default when moving a visible outbound to the top.
        if (positions[0] > 0) result.unshift(result.splice(positions[0], 1)[0]);
        return result;
    }
    return { move };
})();
if (typeof module !== 'undefined') module.exports = DuiOutboundOrder;

if (typeof Vue !== 'undefined') Vue.component('dui-outbound-sort-handle', {
    props: { index: Number, items: Array, label: String },
    template: '<button type="button" class="dui-outbound-sort-handle" :title="label" :aria-label="label" :disabled="items.length < 2" @pointerdown="start" @keydown="key"><a-icon type="drag"></a-icon></button>',
    watch: { items() { this.cancel(); } },
    beforeDestroy() { this.cancel(); },
    methods: {
        key(event) {
            if (event.key === 'Escape') { this.cancel(); return; }
            const offset = event.key === 'ArrowUp' ? -1 : event.key === 'ArrowDown' ? 1 : 0;
            if (!offset) return;
            event.preventDefault();
            this.cancel();
            const target = this.index + offset;
            if (target >= 0 && target < this.items.length) {
                this.$emit('move', this.index, target);
                this.$nextTick(() => {
                    const rows = this.$el.closest('tbody')?.querySelectorAll('tr.ant-table-row');
                    rows?.[target]?.querySelector('.dui-outbound-sort-handle')?.focus();
                });
            }
        },
        start(event) {
            if (event.button !== 0 || event.isPrimary === false || this.items.length < 2) return;
            this.cancel();
            const body = this.$el.closest('tbody');
            if (!body) return;
            event.preventDefault();
            this.$el.focus({ preventScroll: true });
            const scrollParents = [];
            for (let el = body.parentElement; el; el = el.parentElement) {
                if (/(auto|scroll)/.test(getComputedStyle(el).overflowY) && el.scrollHeight > el.clientHeight) scrollParents.push(el);
            }
            if (!scrollParents.includes(document.scrollingElement)) scrollParents.push(document.scrollingElement);
            this._drag = { id: event.pointerId, from: this.index, to: this.index, body,
                x: event.clientX, y: event.clientY, startX: event.clientX, startY: event.clientY,
                moved: false, valid: false, scrollParents, mark: null };
            this.$el.setPointerCapture?.(event.pointerId);
            document.addEventListener('pointermove', this.move, { passive: false });
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
            const rows = [...drag.body.querySelectorAll('tr.ant-table-row')];
            const rect = drag.body.getBoundingClientRect();
            drag.valid = rows.length === this.items.length && drag.x >= rect.left && drag.x <= rect.right &&
                drag.y >= Math.max(0, rect.top - 24) && drag.y <= Math.min(innerHeight, rect.bottom + 24);
            if (drag.mark) drag.mark.classList.remove('dui-outbound-drop-before', 'dui-outbound-drop-after');
            drag.mark = null;
            drag.body.classList.add('dui-outbound-dragging');
            rows[drag.from]?.classList.add('dui-outbound-drag-source');
            if (!drag.valid) return;
            let target = rows.findIndex(row => drag.y < row.getBoundingClientRect().bottom);
            if (target === -1) target = rows.length - 1;
            drag.to = target;
            if (target === drag.from) return;
            drag.mark = rows[target];
            drag.mark.classList.add(target < drag.from ? 'dui-outbound-drop-before' : 'dui-outbound-drop-after');
        },
        tick() {
            const drag = this._drag;
            if (!drag) return;
            if (drag.moved) {
                const tableRect = drag.body.getBoundingClientRect();
                if (drag.x >= tableRect.left && drag.x <= tableRect.right) {
                    for (const parent of drag.scrollParents) {
                        const rect = parent === document.scrollingElement ? { top: 0, bottom: innerHeight } : parent.getBoundingClientRect();
                        const top = Math.max(rect.top, 0), bottom = Math.min(rect.bottom, innerHeight);
                        const direction = drag.y < top + 44 ? -1 : drag.y > bottom - 44 ? 1 : 0;
                        if (!direction) continue;
                        const before = parent.scrollTop;
                        parent.scrollTop += direction * 10;
                        if (parent.scrollTop !== before) break;
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
            if (apply) this.$emit('move', drag.from, drag.to);
        },
        escape(event) { if (event.key === 'Escape') this.cancel(); },
        cancel() {
            if (this._frame) cancelAnimationFrame(this._frame);
            this._frame = null;
            const drag = this._drag;
            this._drag = null;
            if (drag) {
                drag.body.classList.remove('dui-outbound-dragging');
                drag.body.querySelectorAll('tr').forEach(row => row.classList.remove(
                    'dui-outbound-drag-source', 'dui-outbound-drop-before', 'dui-outbound-drop-after'));
                if (this.$el.hasPointerCapture?.(drag.id)) this.$el.releasePointerCapture(drag.id);
            }
            document.removeEventListener('pointermove', this.move);
            document.removeEventListener('pointerup', this.finish);
            document.removeEventListener('pointercancel', this.cancel);
            document.removeEventListener('keydown', this.escape);
            window.removeEventListener('blur', this.cancel);
        }
    }
});
