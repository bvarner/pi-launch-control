import {html, render} from 'https://unpkg.com/lit-html?module';

export default class ControlPanel extends HTMLElement {
    constructor() {
        super();

        this.root = this.attachShadow({mode: 'open'});

        // Events include:

        //  Scale
        //  Camera

        // This panel responds to the following:
        //  Igniter
        //  Sequence <- (Mission Status / Countdown)
        this.eventSource = new EventSource('/events');

        this.mission = {
            Name: 'Untitled',
            Started: false,
            Stopped: false,
            Aborted: false,
            Remaining: -1,
            Downloading: false,
            Igniter: {
                Ready: false,
                Firing: false,
                Recording: false,
                Timestamp: moment(),
            },
        }

    }

    connectedCallback() {
        this.eventSource.addEventListener('Igniter', evt => this.onIgniterEvent(evt));
        this.eventSource.addEventListener( 'Sequence', evt => this.onSequenceEvent(evt));

        const mission = this.mission;

        // Do a fetch to poll the Igniter.
        fetch ('/igniter', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then(function(response) {
            return response.json();
        })
        .then(function(obj) {
            mission.Igniter = obj;
            this.render();
        });

        this.render();
    }

    onIgniterEvent(evt) {
        var igniter = JSON.parse(evt.data);

        this.mission.Igniter = igniter;
        // update the scale graph data with the igniter data.

        this.render();
    }


    onSequenceEvent(evt) {
        var sequence = JSON.parse(evt.data);

        this.mission.Remaining = sequence.Remaining;
        this.mission.Aborted = sequence.Aborted;

        this.render();
    }

    onStart(evt) {
        const mission = this.mission;

        // Assume it works.
        mission.Started = true;
        this.render();

        // Background fetch.
        fetch('/mission/start', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then( function (response) {
            mission.Started = (response.status === 200);
        })
        .catch(function(error) {
            mission.Started = false;
        });

        this.render();
    }

    onStop(evt) {
        const mission = this.mission;

        // Assume it works.
        mission.Stopped = true;
        this.render();

        // Background fetch.
        fetch ('/mission/stop', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then(function(response) {
            mission.Stopped = (response.status === 200);
        })
        .catch(function(response) {
            mission.Stopped = true;
        });

        this.render();
    }

    onAbort(evt) {
        const mission = this.mission;

        mission.Aborted = true;
        this.render();

        fetch ('/mission/abort', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then(function(response) {
            mission.Aborted = (response.status === 200);
        })
        .catch(function(response) {
            mission.Aborted = false;
        });
        this.render();
    }

    onDownload(evt) {
        this.mission.Downloading = true;
        this.render();

        // Fetch the file or get the URL, or whatever.
        fetch('/mission/download?name=' + this.mission.Name, {
            method: 'GET',
            cache: 'no-cache',
        })
        .then(function(response) {
            return response.blob();
        })
        .then( function(blob) {
            return URL.createObjectURL(blob);
        })
        .then(function(url) {
            window.open(url, '_blank');
            URL.revokeObjectURL(url);
        })
        .catch( function(error) {
            console.log(error);
        });

        // Once done,
        this.mission.Downloading = false;
        this.render();
    }

    onNameChange(evt) {
        this.mission.Name = evt.target.value;
    }

    render() {
        const mission = this.mission;

        const template = html`
            <article id="control-panel">
                <slot></slot>
                <style> .nodisplay { display: none; } </style>
                <style> .hoton { color: #ff0000; }</style>
                <section>
                    <div class="${(mission.Downloading ? "nodisplay" : "")}">
                        <label for="mission.name">Mission Name:</label>
                        <input type="text" id="mission.name" ?disabled=${mission.Started} value="${mission.Name}" @change=${(e) => this.onNameChange(e)}/>
                        
                        <button @click=${(e) => this.onStart(e)} ?disabled=${mission.Started || !mission.Igniter.Ready}>Start</button>
                        <button @click=${(e) => this.onAbort(e)} ?disabled=${!mission.Started}>ABORT</button>
                        
                        <label for="mission.remaining">Countdown: </label>
                        <label id="mission.remaining">${(mission.Remaining > 0) ? mission.Remaining : "--"}</label>
                        
                        <label for="mission.igniter">Igniter: </label>
                        <label id="mission.igniter" class="${(mission.Igniter.Firing ? "hoton" : "")}">${(mission.Igniter.Ready ? "Ready " : "Disconnected")} ${(mission.Igniter.Firing ? "HOT" : "")}</label>
                        
                        <button @click=${(e) => this.onStop(e)} ?disabled=${!mission.Started}>Stop</button>
                        <button @click=${(e) => this.onDownload(e)} ?disabled=${mission.Started && mission.Stopped && !mission.Aborted}>Download</button>
                        
                    </div>
                    <div class="${(mission.Downloading ? "" : "nodisplay")}">
                        Please wait for download...
                    </div>
            </article>
        `;
        render(template, this.root);
    }

}

customElements.define("control-panel", ControlPanel);
