import {html, render} from 'https://unpkg.com/lit-html?module';

// ControlPanel contains a Scale and a Camera.
export default class ControlPanel extends HTMLElement {
    constructor() {
        super();

        this.root = this.attachShadow({mode: 'open'});

        this.eventSource = new EventSource('/events');
    }

    connectedCallback() {
        this.render();
    }

    render() {
        const template = html`
            <article id="control-panel">
                <slot></slot> 
            </article>
        `;

        render(template, this.root);
    }

}

customElements.define("control-panel", ControlPanel);