document.body.onload = () => {
    // Inject required scripts
    const toastifyJS = document.createElement("script");
    const toastifyCSS = document.createElement("link");
    const customCSS = document.createElement("style");
    toastifyJS.src = "https://cdnjs.cloudflare.com/ajax/libs/toastify-js/1.12.0/toastify.min.js";
    toastifyCSS.href = "https://cdnjs.cloudflare.com/ajax/libs/toastify-js/1.12.0/toastify.min.css";
    customCSS.innerHTML = `
    .SSDBANNER {
        top: 0 !important;
        left: 0 !important;
    }
    .BLACKBG {
        background: black !important;
        background-color: black !important;
        color: white;
        display: flex;
        justify-content: center;
        align-items: center;
    }
    `
    document.head.appendChild(toastifyJS);
    document.head.appendChild(toastifyCSS);
    document.head.appendChild(customCSS);

    // Initialise the extension
    init();
}

var firstNotification = true;

function notifyUser(text) {
    function doNotify() {
        console.log("Showing new toast for \"" + text + "\"");
        var data = `window.Toastify({
            text: \`${text}\`,
            duration: 3000,
            newWindow: true,
            close: false,
            duration: -1,
            className: "SSDBANNER",
            style: {
                background: "firebrick",
                color: "white",
                position: "fixed",
                width: "100%",
                textAlign: "center",
                zIndex: "9999999"
            },
            onClick: function(){} // Callback after click
        }).showToast();`
        const inject = document.createElement("script");
        inject.innerHTML = data;
        document.body.appendChild(inject);
    }
    if (firstNotification) {
        // Give some time to make sure the toasts lib has been loaded
        setTimeout(() => {
            firstNotification = false;
            doNotify();
        }, 1e3);
    } else {
        doNotify();
    }
}

function showLocalServerConnectError() {
    return notifyUser("Failed to connect to local Soap2Day Mirror server, please ensure it is running!");
}

function init() {
    if (location.host.includes("soap2day") || location.host.includes("s2dfree")) {
        fetch("http://localhost:8918/ping")
        .then(r => {
            if (r.status.toString()[0] !== "2" && r.status !== 304) return showLocalServerConnectError();
            if (document.getElementById("divPlayerSelect") === null) return;
            document.body.classList.add("BLACKBG");
            document.body.innerHTML = "<h1>Loading...</h1>";
            fetch("http://localhost:8918/GetPlayer", {
                method: "POST",
                body: JSON.stringify({
                    page: `https://soap2day.mx${location.pathname}`
                })
            })
            .then(r => r.text())
            .then(r => {
                if ((r.includes("http") && r.includes("://")) || r.includes("USECACHESERVER")) {
                    if (r.startsWith("USECACHESERVER")) {
                        r = "http://localhost:8918/GetVideo?p=" + encodeURIComponent(r.split("::")[1])
                    }
                    location = r;
                } else {
                    notifyUser("Failed to fetch video for this movie, please try again later")
                }
            })
        })
        .catch(e => {
            console.error("Failed to connect to Soap2Day Mirror server, error:", e);
            return showLocalServerConnectError();
        })
    }
}