import {html, render} from 'https://unpkg.com/lit-html?module';

export default class ScalePanel extends HTMLElement {
    constructor() {
        super();

        this.root = this.attachShadow({mode: 'open'});
        this.scaleReading = "unknown";

        this.eventSource = new EventSource('/events');
    }

    connectedCallback() {
        this.render();
        this.eventSource.addEventListener('Scale', e => this.onScaleSample(e))
    }

    onScaleSample(evt) {
        var sample = JSON.parse(evt.data);
        this.scaleReading = sample.Volt0;

        this.render();
    }

    render() {
        const reading = this.scaleReading;
        const template = html`
            <section>
                <div id="chart"></div>
                <div>
                    <label>${reading}</label>
                </div>
            </section>
        `;

        render(template, this.root);
    }
}

customElements.define("scale-panel", ScalePanel);