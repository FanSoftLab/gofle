// frontend/src/input_history.ts
// Per-namespace input history controller for the search field. Attaches to a
// single input element and shows a dropdown of previously entered values,
// navigable with ArrowUp/ArrowDown and selectable with Enter or a mouse click.
// Individual entries can be deleted and the whole history cleared.

import { t } from './i18n';
import { showToast } from './toast';

export interface InputHistoryOptions {
    namespace: string;
    getApi: () => any;
    historyButton?: HTMLElement | null;
    onExecute?: (value: string) => void;
}

export class InputHistory {
    private input: HTMLInputElement;
    private opts: InputHistoryOptions;
    private items: string[] = [];
    private filtered: string[] = [];
    private isOpen = false;
    private navIndex = -1; // -1 = draft mode, 0..n-1 = index into filtered
    private draft = '';
    private dropdown: HTMLDivElement | null = null;

    constructor(input: HTMLInputElement, opts: InputHistoryOptions) {
        this.input = input;
        this.opts = opts;
    }

    // load fetches the persisted history for this namespace from the backend.
    async load() {
        const api = this.opts.getApi?.();
        if (api && api.GetInputHistory) {
            try {
                const items = await api.GetInputHistory(this.opts.namespace);
                if (Array.isArray(items)) this.items = items;
            } catch (err) {
                showToast(String((err as any)?.message ?? err), 'error');
            }
        }
    }

    // attach wires up the input + optional history button listeners.
    attach() {
        this.input.addEventListener('input', this.onInput);
        this.input.addEventListener('keydown', this.onKeydown);
        this.input.addEventListener('blur', this.onBlur);
        if (this.opts.historyButton) {
            this.opts.historyButton.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.toggle();
            });
        }
    }

    // commit records a value into history (skipping empty/whitespace), then
    // refreshes the local copy so a subsequent open shows the new entry.
    async commit(value: string) {
        const api = this.opts.getApi?.();
        if (api && api.AddInputHistory) {
            try {
                await api.AddInputHistory(this.opts.namespace, value);
            } catch (err) {
                showToast(String((err as any)?.message ?? err), 'error');
            }
        }
        await this.load();
    }

    // removeItem deletes a single entry from the backend history for this
    // namespace, then refreshes the dropdown if it is still open.
    async removeItem(value: string) {
        const api = this.opts.getApi?.();
        if (api && api.RemoveInputHistoryItem) {
            try {
                await api.RemoveInputHistoryItem(this.opts.namespace, value);
            } catch (err) {
                showToast(String((err as any)?.message ?? err), 'error');
            }
        }
        this.items = this.items.filter((i) => i !== value);
        this.recomputeFiltered();
        if (this.isOpen) this.render();
    }

    // clearAll wipes the whole namespace history on the backend and closes the
    // dropdown.
    async clearAll() {
        const api = this.opts.getApi?.();
        if (api && api.ClearInputHistory) {
            try {
                await api.ClearInputHistory(this.opts.namespace);
            } catch (err) {
                showToast(String((err as any)?.message ?? err), 'error');
            }
        }
        this.items = [];
        this.close();
    }

    private onInput = () => {
        const query = this.input.value;
        if (query.trim() === '') {
            this.close();
            return;
        }
        this.recomputeFiltered();
        this.navIndex = -1;
        this.draft = query;
        this.open();
    };

    private onBlur = () => {
        // Delay so a mousedown on a dropdown item can register before we close.
        setTimeout(() => this.close(), 120);
    };

    private onKeydown = (e: KeyboardEvent) => {
        if (e.key === 'ArrowUp') {
            e.preventDefault();
            this.handleArrowUp();
        } else if (e.key === 'ArrowDown') {
            e.preventDefault();
            this.handleArrowDown();
        } else if (e.key === 'Enter') {
            e.preventDefault();
            this.close();
            const v = this.input.value;
            this.opts.onExecute?.(v);
        }
    };

    private handleArrowUp() {
        if (!this.isOpen) this.open();
        if (this.filtered.length === 0) return;
        if (this.navIndex === -1) {
            this.draft = this.input.value;
            this.navIndex = 0;
        } else if (this.navIndex < this.filtered.length - 1) {
            this.navIndex++;
        }
        this.setInput(this.filtered[this.navIndex]);
        this.render();
    }

    private handleArrowDown() {
        if (!this.isOpen) this.open();
        if (this.filtered.length === 0) {
            this.close();
            return;
        }
        if (this.navIndex > 0) {
            this.navIndex--;
        } else {
            this.navIndex = -1;
        }
        const v = this.navIndex === -1 ? this.draft : this.filtered[this.navIndex];
        this.setInput(v);
        this.render();
    }

    private recomputeFiltered() {
        const query = this.input.value;
        if (query.trim() === '') {
            this.filtered = this.items.slice();
        } else {
            const q = query.toLowerCase();
            this.filtered = this.items.filter((i) => i.toLowerCase().includes(q));
        }
    }

    private toggle() {
        if (this.isOpen) {
            this.close();
        } else {
            this.recomputeFiltered();
            this.navIndex = -1;
            this.draft = this.input.value;
            this.open();
        }
    }

    private open() {
        this.isOpen = true;
        this.render();
    }

    private close() {
        this.isOpen = false;
        this.navIndex = -1;
        this.draft = '';
        this.filtered = [];
        this.removeDropdown();
    }

    // setInput writes the value WITHOUT dispatching an input event so navigating
    // through history does not re-filter or re-trigger live search.
    private setInput(v: string) {
        this.input.value = v;
    }

    private render() {
        if (!this.isOpen) {
            this.removeDropdown();
            return;
        }
        const parent = this.input.parentElement;
        if (!parent) {
            this.removeDropdown();
            return;
        }
        if (!this.dropdown) {
            this.dropdown = document.createElement('div');
            this.dropdown.className = 'input-history-dropdown';
        }
        this.dropdown.innerHTML = '';

        // Nothing to show: keep the dropdown out of the way entirely.
        if (this.filtered.length === 0) {
            this.removeDropdown();
            return;
        }

        this.filtered.forEach((item, idx) => {
            const el = document.createElement('div');
            el.className = 'input-history-item' + (idx === this.navIndex ? ' active' : '');

            const text = document.createElement('span');
            text.className = 'input-history-text';
            text.textContent = item;
            el.appendChild(text);

            const del = document.createElement('button');
            del.className = 'input-history-delete';
            del.type = 'button';
            del.title = t('history_delete_item');
            del.textContent = '×';
            del.addEventListener('mousedown', (e) => {
                // Keep the input focused (preventDefault) so the dropdown does
                // not close on blur, and stop the item-select handler.
                e.preventDefault();
                e.stopPropagation();
                this.removeItem(item);
            });
            el.appendChild(del);

            el.addEventListener('mousedown', (e) => {
                e.preventDefault();
                this.setInput(item);
                this.navIndex = -1;
                this.draft = item;
                this.close();
                this.input.focus();
            });
            this.dropdown!.appendChild(el);
        });

        const clear = document.createElement('div');
        clear.className = 'input-history-clear';
        clear.textContent = t('history_clear_all');
        clear.addEventListener('mousedown', (e) => {
            e.preventDefault();
            this.clearAll();
        });
        this.dropdown.appendChild(clear);

        parent.appendChild(this.dropdown);
    }

    private removeDropdown() {
        if (this.dropdown && this.dropdown.parentElement) {
            this.dropdown.parentElement.removeChild(this.dropdown);
        }
    }
}
