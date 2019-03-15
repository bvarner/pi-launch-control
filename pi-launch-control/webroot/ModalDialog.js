import {html, render} from 'https://unpkg.com/lit-html?module';

export default class ModalDialog extends HTMLElement {

    static get observedAttributes() {
        return [ "visible", "title" ];
    }

    constructor() {
        super();

        this.root = this.attachShadow({mode: 'open'});

        this.visible = false;
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (name === 'title' && this.root) {
            let title = this.root.querySelector(".title");
            if (title) {
                title.textContent = newValue;
            }
        }

        if (name === 'visible' && this.root) {
            let wrapper = this.root.querySelector('.wrapper');
            if (wrapper) {
                if (newValue === null) {
                    wrapper.classList.remove('visible');
                } else {
                    this.root.querySelector('.wrapper').classList.add('visible');
                }
            }
        }
    }

    connectedCallback() {
        this.render();
        this.attachEventHandlers();
    }

    get title() {
        return this.getAttribute('title');
    }

    set title(title) {
        this.setAttribute('title', title);
    }

    get visible() {
        return this.hasAttribute('visible');
    }

    set visible(visible) {
        if (visible) {
            this.setAttribute("visible", "");
        } else {
            this.removeAttribute("visible");
        }
    }

    render() {
        const wrapperClass = this.visible ? "wrapper visible" : "wrapper";
        const template = html`
            <style>
                .wrapper {
                    position: fixed;
                    left: 0;
                    top: 0;
                    width: 100%;
                    height: 100%;
                    background-color: #b0b0b0;
                    opacity: 0;
                    visibility: hidden;
                    transform: scale(1.1);
                    transition: visibility 0s linear .25s,opacity .25s 0s,transform .25s;
                    z-index: 1;
                }
                .visible {
                    opacity: 1;
                    visibility: visible;
                    transform: scale(1);
                    transition: visibility 0s linear 0s,opacity .25s 0s,transform .25s;
                }
                .modal {
                    font-family: Helvetica;
                    font-size: 1em;
                    padding: 0.7em 0.7em 0.35em 0.7em;
                    background-color: #fff;
                    position: absolute;
                    top: 50%;
                    left: 50%;
                    transform: translate(-50%,-50%);
                    border-radius: 0.15em;
                    min-width: 22em;
                }
                .title {
                    font-size: 1.25em;
                }
                .button-container {
                    text-align: right;
                }
                button {
                    min-width: 6em;
                    background-color: #848e97;
                    border-color: #848e97;
                    border-style: solid;
                    border-radius: 0.15em;
                    padding: 0.2em;
                    color: #fff;
                    cursor: pointer;
                }
                button:hover {
                    background-color: #6c757d;
                    border-color: #6c757d;
                }
            </style>
            <div class="${wrapperClass}">
                <div class="modal">
                    <span class="title">${this.title}</span>
                    <div class="content">
                        <slot></slot>
                    </div>
                    <div class="button-container">
                        <button class="ok">Ok</button>
                        <button class="cancel">Cancel</button>
                    </div>
                </div>
            </div>
        `;

        render(template, this.root);
    }

    attachEventHandlers() {
        const okButton = this.root.querySelector('.ok');
        okButton.addEventListener('click', e => {
            this.dispatchEvent(new CustomEvent("modal-ok"));
            this.removeAttribute('visible');
        });

        const cancelButton = this.root.querySelector('.cancel');
        cancelButton.addEventListener('click', e => {
            this.dispatchEvent(new CustomEvent("modal-cancel"));
            this.removeAttribute('visible');
        });
    }
}

customElements.define("modal-dialog", ModalDialog);
