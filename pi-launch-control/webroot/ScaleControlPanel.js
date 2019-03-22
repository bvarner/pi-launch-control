import {html, render} from 'https://unpkg.com/lit-html?module';

export default class ScaleControlPanel extends HTMLElement {
    constructor() {
        super();

        this.root = this.attachShadow({mode: 'open'});

        this.scale = {
            Initialized: false,
            Calibrated: false,
            Recording: false,
            Timestamp: moment(),
            Volt0: '',
            Volt0Mass: 'Please Calibrate',
            Volt1: '',
            Volt1Maxx: 'Please Calibrate',
        };
    }

    connectedCallback() {
        this.render();

        const controlpanel = document.querySelector('control-panel');
        controlpanel.eventSource.addEventListener('Scale', evt => this.onScaleSample(evt));

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
        var samples = JSON.parse(evt.data);

        // The scale is an array of measurements taken since the last time a measurement was taken.
        // We average ~40 / second.
        // so for a 30 second graph, we'd need 30 * 40, 1200 data points.

        // We report the latest (tail) of the samples recieved as the current state of the scale.
        if (samples.length > 0) {
            this.scale = samples[samples.length - 1];
            this.scale.Volt0Mass = this.scale.Volt0Mass !== null ? this.scale.Volt0Mass : 0;

            const scale = this.scale;

            samples.forEach(function (sample, idx) {
                // If we see a move from
                if (scale.Recording == false && sample.recording == true) {
                    scale.Recording = true;

                    scaleChart.data.datasets[0].data = [];
                    scaleChart.data.datasets[1].data = [];
                }

                const ts = moment(Math.floor(sample.Timestamp / 1000000));
                scaleChart.data.datasets[0].data.push({x: ts, y: sample.Volt0});
                scaleChart.data.datasets[1].data.push({x: ts, y: sample.Volt0Mass});
            });

            // Shift the chart dataset if we're not recording
            if (scale.Recording == false) {
                if (scaleChart.data.datasets[0].data.length > 1200) {
                    scaleChart.data.datasets[0].data = scaleChart.data.datasets[0].data.slice(scaleChart.data.datasets[0].data.length - 1200);
                }
                if (scaleChart.data.datasets[1].data.length > 1200) {
                    scaleChart.data.datasets[1].data = scaleChart.data.datasets[1].data.slice(scaleChart.data.datasets[1].data.length - 1200);
                }
            }

            scaleChart.update();
        }

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
                <div>
                    <label>${scale.Volt0Mass}</label>
                    <label>${scale.Recording}</label>
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

customElements.define("scale-control-panel", ScaleControlPanel);