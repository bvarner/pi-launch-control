import {html, render} from 'https://unpkg.com/lit-html?module';

export default class ScalePanel extends HTMLElement {
    constructor() {
        super();

        this.root = this.attachShadow({mode: 'open'});

        this.scale = {
            Initialized: false,
            Calibrated: false,
            Volt0: '',
            Volt0Mass: 'Please Calibrate',
            Volt1: '',
            Volt1Maxx: 'Please Calibrate',
        };

        this.eventSource = new EventSource('/events');
    }

    connectedCallback() {
        this.render();

        // TODO: Move the EventSource up to a component that holds all these other things in a slot.
        this.eventSource.addEventListener('Scale', evt => this.onScaleSample(evt));

        const modal = document.querySelector('modal-dialog');
        const scale = this.scale;

        modal.addEventListener('modal-ok', _ => {
            const mass = modal.querySelector('input#mass');

            fetch('/scale/calibrate?mass=' + mass.value, {
                method: 'POST',
                cache: "no-cache",
            })
            .then(function (response) {
                return response.json();
            })
            .then(function(obj) {
                if (obj.Calibrated) {
                    const event = new CustomEvent('scale-calibrated', {
                        detail: {},
                        bubbles: true
                    });
                    dispatchEvent(event);
                }
            });
        });
    }

    onScaleSample(evt) {
        this.scale = JSON.parse(evt.data);;
        this.render();
    }

    onTare(evt) {
        fetch('/scale/tare', {
            cache: "no-cache",
        })
        .then(function (response) {
            return response.json();
        })
        .then(function(obj) {
            console.log(JSON.stringify(obj));
        });
    }

    onCalibrate(evt) {
        const modal = document.querySelector('modal-dialog');
        modal.visible = true;
    }

    render() {
        const scale = this.scale;
        const template = html`
            <section>
                <div id="chart">
                </div>
                <div>
                    <label>${scale.Volt0}</label>
                </div>
                <div>
                    <button ?disabled=${!scale.Initialized} @click=${(e) => this.onTare(e)}>Tare</button>
                    <button ?disabled=${!scale.Initialized} @click=${(e) => this.onCalibrate(e)}>Calibrate</button>
                </div>
            </section>
        `;

        render(template, this.root);
    }
}

customElements.define("scale-panel", ScalePanel);