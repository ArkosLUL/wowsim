import { Tooltip } from 'bootstrap';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { element, ref } from 'tsx-vanilla';

import { Component } from "./component";

export interface CopyButtonConfig {
	getContent: () => string,
	extraCssClasses?: string[],
	text?: string,
	tooltip?: string,
}

export class CopyButton extends Component {
  private readonly config: CopyButtonConfig;

  constructor(parent: HTMLElement, config: CopyButtonConfig) {
		const btnRef = ref<HTMLButtonElement>();
		const buttonElem = (
			<button
				className={`btn ${config.extraCssClasses?.join(' ') ?? ''}`}
				ref={btnRef}
			>
				<i className="fas fa-copy me-1" />{config.text ?? 'Copy to Clipboard'}
			</button>
		)

    super(parent, 'copy-button', buttonElem as HTMLElement);
    this.config = config;

		const button = btnRef.value!
		let clicked = false
		button.addEventListener('click', _event => {
			if (clicked) return

			const data = this.config.getContent()
			if (navigator.clipboard != undefined) {
				navigator.clipboard.writeText(data);
			} else if (!copyBySelection(data, button)) {
				alert(data);
				return;
			}
			clicked = true
			const originalContent = button.innerHTML;
			button.innerHTML = '<i class="fas fa-check me-1"></i>Copied';
			setTimeout(() => {
				button.innerHTML = originalContent;
				clicked = false
			}, 1500);
		});

		if (config.tooltip) {
			Tooltip.getOrCreateInstance(button, {title: config.tooltip});
		}
  }
};

// For plain http, e.g. the sim opened by LAN address, where browsers leave out navigator.clipboard.
// The textarea goes next to the button: a Bootstrap modal's focus trap pulls focus out of anything outside it.
function copyBySelection(text: string, anchor: HTMLElement): boolean {
	const field = document.createElement('textarea');
	field.value = text;
	field.readOnly = true;
	field.style.position = 'fixed';
	field.style.top = '0';
	field.style.left = '0';
	field.style.opacity = '0';
	field.style.fontSize = '12pt'; // smaller and iOS zooms in on focus
	anchor.after(field);
	try {
		field.focus();
		field.select();
		field.setSelectionRange(0, text.length); // iOS ignores select()
		return document.execCommand('copy');
	} catch {
		return false;
	} finally {
		field.remove();
		anchor.focus();
	}
}
