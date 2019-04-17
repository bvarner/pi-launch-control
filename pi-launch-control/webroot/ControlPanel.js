import {html, render} from 'https://unpkg.com/lit-html?module';

export default class ControlPanel extends HTMLElement {
    constructor() {
        super();

        this.root = this.attachShadow({mode: 'open'});

        // Events include:
        //  Scale
        //  Camera
        //  Igniter
        //  Sequence <- (Mission Status / Countdown)
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
