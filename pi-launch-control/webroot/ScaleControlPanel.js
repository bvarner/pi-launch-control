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
            Volt0Mass: null,
            Volt1: '',
            Volt1Mass: null,
        };

        this.retention = 10;
    }

    getVolt0Mass() {
        return this.scale.Volt0Mass != null ? Math.floor(this.scale.Volt0Mass * 100) / 100 : 0;
    }

    connectedCallback() {
        this.render();

        const controlpanel = document.querySelector('control-panel');
        controlpanel.eventSource.addEventListener('Scale', evt => this.onScaleSample(evt));
        controlpanel.eventSource.addEventListener('open', evt => this.eventStreamConnected(evt));

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

    eventStreamConnected(evt) {
        const scaleControlPanel = this;

        fetch('/scale', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then(function(response) {
            return response.json();
        })
        .then(function(obj) {
            scaleControlPanel.scale = obj;
            scaleControlPanel.render();
        });
    }


    onTare(evt) {
        const scaleControlPanel = this;

        fetch('/scale/tare', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then(function(response) {
            return response.json();
        })
        .then(function(obj) {
            scaleControlPanel.scale = obj;
            scaleControlPanel.render();
        });
    }


    onScaleSample(evt) {
        var samples = JSON.parse(evt.data);

        // The scale is an array of measurements taken since the last time a measurement was taken.

        // We report the latest (tail) of the samples recieved as the current state of the scale.
        if (samples.length > 0) {
            this.scale = samples[samples.length - 1];

            const scale = this.scale;

            samples.forEach(function (sample, idx) {
                // If we see a move from
                if (scale.Recording == false && sample.Recording == true) {
                    scale.Recording = true;

                    missionChart.data.datasets[0].data = [];
                    missionChart.data.datasets[1].data = [];
                    missionChart.data.datasets[2].data = [];
                }

                const ts = moment(Math.floor(sample.Timestamp / 1000000));
                missionChart.data.datasets[0].data.push({x: ts, y: sample.Volt0});
                missionChart.data.datasets[1].data.push({x: ts, y: sample.Volt0Mass});
                missionChart.data.datasets[2].data.push({x: ts, y: null});
            });

            // Shift the chart dataset if we're not recording
            if (scale.Recording == false) {
                // Find the index of the data element representing now() - retention.
                var d = missionChart.data.datasets[0].data = missionChart.data.datasets[0].data;
                var retainAfter = moment(d[d.length - 1].x).subtract(this.retention, 'seconds');

                if (d[0].x <= retainAfter) {
                    var cutIdx = 0;
                    for (var i = 0; i < d.length; i++) {
                        if (d[i].x >= retainAfter) {
                            cutIdx = i;
                            break;
                        }
                    }

                    if (cutIdx > 0) {
                        missionChart.data.datasets[0].data = missionChart.data.datasets[0].data.slice(cutIdx);
                        missionChart.data.datasets[1].data = missionChart.data.datasets[1].data.slice(cutIdx);
                        missionChart.data.datasets[2].data = missionChart.data.datasets[2].data.slice(cutIdx);
                    }
                }
            }

            missionChart.update();
        }

        this.render();
    }

    onCalibrate(evt) {
        const modal = document.querySelector('modal-dialog');
        modal.visible = true;
    }

    onRetentionChange(evt) {
        this.retention = evt.target.value;
        this.render();
    }

    render() {
        const scale = this.scale;
        const mass = this.getVolt0Mass();
        const retention = this.retention;
        const template = html`
            <style> .nodisplay { display: none; } </style>
            <section class="${(scale.Initialized ? "" : "nodisplay")}">
                <div>
                    <label>${mass}</label>
                    <label>${scale.Recording}</label>
                    <span id="scale-retention" class="${(scale.Recording ? "nodisplay" : "")}" >
                        <label for="retention">Graph <input type="number" id="scale-retain-seconds" value="${retention}" @change=${(e) => this.onRetentionChange(e)}/> seconds</label>
                    </span>
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