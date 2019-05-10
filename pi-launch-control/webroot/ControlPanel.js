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
        //  Mission <- (Mission Status / Countdown)
        this.eventSource = new EventSource('/events');

        this.mission = {
            Name: 'Untitled',
            Running: false,
            Aborted: false,
            Complete: false,
            Clock: NaN,
            Igniter: {
                Ready: false,
                Firing: false,
                Recording: false,
                Timestamp: moment(),
            },
        }

        this.clockSynch = false;

    }

    connectedCallback() {
        this.eventSource.addEventListener('Igniter', evt => this.onIgniterEvent(evt));
        this.eventSource.addEventListener( 'Mission', evt => this.onMissionEvent(evt));

        const controlPanel = this;

        // Do a fetch to poll the Igniter.
        fetch ('/igniter', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then(function(response) {
            return response.json();
        })
        .then(function(obj) {
            controlPanel.mission.Igniter = obj;
            controlPanel.render();
        });

        this.render();
    }

    onIgniterEvent(evt) {
        var igniter = JSON.parse(evt.data);

        this.mission.Igniter = igniter;

        this.render();
    }


    onMissionEvent(evt) {
        var mission = JSON.parse(evt.data);

        this.mission.Clock = mission.Clock;
        this.mission.Aborted = mission.Aborted;
        if (this.mission.Aborted) {
            this.mission.Running = false;
        }

        if (mission.Complete && !this.mission.Complete) {
            this.mission.Complete = mission.Complete;
            this.onCompleteMission(evt);
        }
        this.mission.Complete = mission.Complete;

        this.render();
    }

    onStart(evt) {
        const controlPanel = this;
        // Prevent double-click.
        evt.target.setAttribute("disabled", "true");

        // Background fetch.
        fetch('/mission/start', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then( function (response) {
            controlPanel.mission.Running = (response.status === 200);
        })
        .catch(function(error) {
            controlPanel.mission.Running = false;
        });

        this.render();
    }

    onCompleteMission(evt) {
        const controlPanel = this;

        // Fetch the file or get the URL, or whatever.
        window.open('/mission/download?name=' + controlPanel.mission.Name, "_blank", "", false);

        controlPanel.mission.Name = 'Untitled';
        controlPanel.mission.Running = false;
        controlPanel.mission.Aborted = false;
        controlPanel.mission.Complete = false;
        controlPanel.mission.Remaining = -1;
        controlPanel.mission.Clock = NaN;

        controlPanel.render();

        this.render();
    }

    onAbort(evt) {
        const controlPanel = this;

        controlPanel.mission.Running = false;
        controlPanel.mission.Aborted = true;
        controlPanel.mission.Complete = false;
        controlPanel.render();

        fetch ('/mission/abort', {
            method: 'GET',
            cache: 'no-cache',
        })
        .then(function(response) {
            controlPanel.mission.Running = false;
            controlPanel.mission.Aborted = (response.status === 200 || response.status === 417);
            controlPanel.mission.Clock = NaN;
            controlPanel.render();
        })
    }

    onNameChange(evt) {
        this.mission.Name = evt.target.value;
    }

    render() {
        const mission = this.mission;

        let igstring = "Disconnect";
        if (mission.Igniter.Firing) {
            igstring = "HOT";
        } else if (mission.Igniter.Ready) {
            igstring = "Ready";
        }
        const IgniterState = igstring;


        const template = html`
            <article id="control-panel">
                <slot></slot>
                <style> .nodisplay { display: none; } </style>
                <style> .hoton { color: #ff0000; }</style>
                <section>
                    <div>
                        <label for="mission.name">Mission Name:</label>
                        <input type="text" id="mission.name" ?disabled=${mission.Running} value="${mission.Name}" @change=${(e) => this.onNameChange(e)}/>
                        
                        <button @click=${(e) => this.onStart(e)} ?disabled=${!mission.Igniter.Ready || mission.Running}>Start</button>
                        <button @click=${(e) => this.onAbort(e)} ?disabled=${!mission.Running || mission.Aborted }>ABORT</button>
                        
                        <label for="mission.clock">Clock: </label>
                        <label id="mission.clock">${isNaN(mission.Clock) ? "--" : mission.Clock}</label>
                        
                        <label for="mission.igniter">Igniter: </label>
                        <label id="mission.igniter" class="${(mission.Igniter.Firing ? "hoton" : "")}">${IgniterState}</label>
                    </div>
                </section>
            </article>
        `;
        render(template, this.root);
    }

}

customElements.define("control-panel", ControlPanel);
