import ControlPanel from './ControlPanel.js';
import ScaleControlPanel from './ScaleControlPanel.js';
import CameraPanel from './CameraPanel.js';
import ModalDialog from './ModalDialog.js';

class App {
    constructor() {
        console.log("Constructing app...");
        fetch ("/clock?tstamp=" + Math.round(Date.now() / 1000), {
            method: 'POST',
            cache: 'no-cache'
        })
        .then(function(response) {
            if (response.status === 200) {
                console.log("Clock Synchronized to the best of our ability.")
            } else {
                console.log("Clock Synch FAILED.")
            }
        })
    }
}
new App();
