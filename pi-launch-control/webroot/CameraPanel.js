import {html, render} from 'https://unpkg.com/lit-html?module';

export default class CameraPanel extends HTMLElement {
    constructor() {
        super();

        this.root = this.attachShadow({mode: 'open'});
        this.camera = {
            Initialized: false,
            Recording: false,
        }
    }

    connectedCallback() {
        this.render();

        const controlpanel = document.querySelector('control-panel');
        controlpanel.eventSource.addEventListener('Camera', evt => this.onCamera());

        const cameraPanel = this;

        fetch('/camera/status', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then(function(response) {
            return response.json();
        })
        .then(function(obj) {
            cameraPanel.camera = obj;
            cameraPanel.render();
        });
    }

    onCamera(evt) {
        // TODO: Handle Camera Events.
    }

    render() {
        const camera = this.camera;
        const template = html`
            <section>
                <div>
                    <img src="/camera"/>
                </div>
                <div>
                    ${camera.Initialized} : ${camera.Recording}
                </div>
                <slot>
                </slot>
            </section>
        `;

        render(template, this.root);
    }
}

customElements.define('camera-panel', CameraPanel);