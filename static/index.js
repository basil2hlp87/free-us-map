// https://switch2osm.org/using-tiles/getting-started-with-leaflet/

// Default zoom level.
// (eventually) Zoom level above which no searches will be kicked off.
const DEFAULT_ZOOM_THRESHOLD = 11;
const DEFAULT_LON = -93.2624;
const DEFAULT_LAT = 44.9343;

// Initialize map //

var startLat = DEFAULT_LAT;
var startLon = DEFAULT_LON;
var startZm = DEFAULT_ZOOM_THRESHOLD;

// Use parameters from the URL to center the map if they are set.
var fetchCurrentLocation = true;
const queryString = window.location.search;
if (queryString) {
    const params = new URLSearchParams(queryString);

    startLat = params.get("lat") || startLat;
    startLon = params.get("lng") || startLon;
    startZm = params.get("zm") || startZm;

    fetchCurrentLocation = false;
}

// initialize Leaflet
var map = L.map('map').setView({lon: startLon, lat: startLat}, startZm);

// Center the map on current location if available and we haven't used the
// params from the URL to set the current location.
if (fetchCurrentLocation) {
    map.locate({setView: true, maxZoom: startZm});
    map.on('locationfound', e => map.setView(e.latlng));
}

// add the OpenStreetMap tiles
L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19,
    attribution: '&copy; <a href="https://openstreetmap.org/copyright">OpenStreetMap contributors</a>'
}).addTo(map);

// show the scale bar on the lower left corner
L.control.scale().addTo(map);

map.on('moveend', e => afterMoveZoomEnd());
map.on('zoomend', e => afterMoveZoomEnd());

// Trigger popup box for creating a point after clicking on the map.
map.on('click', e => createPoint(e));

// Dismiss the introductory info box after the X icon is clicked.
document.getElementById("close-intro-msg").addEventListener('click', e => {
    document.getElementById("intro-msg").hidden = true;
});

// Check if the user if verified and reflect that in the icon in the top right.
var isVerified = false;
checkIfVerified();

// Submit the verification request when we click the submit button.
document.getElementById("verify-submit").addEventListener('click', submitVerificationRequest);

// Show the verification info box if the verified icon is clicked.
document.getElementById("login-indicator").addEventListener('click', e => {
    const msgBox = document.getElementById("verify-msg")
    msgBox.hidden = !msgBox.hidden;
});

// Dismiss the verification info box after the X icon is clicked.
document.getElementById("close-verify-msg").addEventListener('click', e => {
    document.getElementById("verify-msg").hidden = true;
});

// Load all points into the map.
getVisibilePoints();

// Function declarations //

function submitVerificationRequest(e) {
    var httpRequest = new XMLHttpRequest();

    httpRequest.onreadystatechange = () => {
        if (httpRequest.readyState === XMLHttpRequest.DONE) {
            if (httpRequest.status === 200) {
                document.getElementById("verify-text").innerHTML = "👍 Click on the link sent to your email address to start adding points!";
            } else {
                console.log("There was a problem with the request.");
            }
        }
    };

    var body = {
        email: document.getElementById("verify-email").value,
        current_url: location.toString(),
    };

    httpRequest.open('POST', '/api/v1/send_verification');
    httpRequest.send(JSON.stringify(body));
}

function afterMoveZoomEnd() {
    // When the map moves update the address bar with the new location so that
    // the map position is retained across refreshes and can be shared.
    updateAddressBarWithCurrentLocation();

    // Add event handler to trigger new search when the map moves.
    // TODO: (optimization) Don't search if the new area is within the
    // previous area.
    getVisibilePoints();
}

function updateAddressBarWithCurrentLocation() {
    const center = map.getCenter();
    const newQueryString = `lat=${center.lat}&lng=${center.lng}&zm=${map.getZoom()}`
    const newUrl = `${window.location.protocol}//${window.location.host}?${newQueryString}`;

    history.replaceState(null, null, newUrl);
}

function checkIfVerified() {
    var httpRequest = new XMLHttpRequest();

    httpRequest.onreadystatechange = () => {
        if (httpRequest.readyState === XMLHttpRequest.DONE) {
            if (httpRequest.status === 200) {
                var resp = JSON.parse(httpRequest.responseText);
                if (resp.verified) {
                    setUserIsVerified();
                }
            } else {
                console.log("There was a problem with the request.");
            }
        }
    };

    httpRequest.open('GET', '/api/v1/is_verified');
    httpRequest.send();
}

function setUserIsVerified() {
    isVerified = true;
    document.getElementById("login-indicator").innerHTML = "✅";
}

function getVisibilePoints() {
    var httpRequest = new XMLHttpRequest();

    httpRequest.onreadystatechange = () => {
        if (httpRequest.readyState === XMLHttpRequest.DONE) {
            if (httpRequest.status === 200) {
                var resp = JSON.parse(httpRequest.responseText);
                addNewPointsToMap(resp);
            } else {
                console.log("There was a problem with the request.");
            }
        }
    };

    var corners = map.getBounds();
    var body = { NE: corners.getNorthEast(), SW: corners.getSouthWest() };

    httpRequest.open('POST', '/api/v1/points');
    httpRequest.send(JSON.stringify(body));
}

// Only add points to the map if they're not already there. Add the new points
// to our tracker so we don't add the points twice.
var pointsOnMap = {};
function addNewPointsToMap(allPts) {
    allPts.filter(pt => !pointsOnMap[pt.properties.point_id]).forEach(pt => {
        const ptId = pt.properties.point_id;
        pointsOnMap[ptId] = true;

        var createdAt = new Date(pt.properties.created_at)
        var body = `<h6>${pt.properties.message}</h6><p>${createdAt.toLocaleTimeString('en-US', {timeZoneName: 'short' })} ${createdAt.toDateString()}</p>`;

        // Add buttons for deleting if you created the point.
        if (pt.can_delete) {
            body = body + `<button id="delete-btn-${ptId}" type="submit" class="btn btn-danger btn-sm">Remove</button>`;
        // Add buttons for up/down-voting if you did not create the point.
        } else {
            body = body + `
                <button id="upvote-btn-${ptId}" type="button" class="btn btn-outline-secondary btn-sm">👍 Helpful</button>
                <button id="downvote-btn-${ptId}" type="button" class="btn btn-outline-secondary btn-sm">👎 Not helpful</button>
            `
        }

        var iconDiv = divIconFor(pt.properties.icon, opacityForAge(createdAt));
        var newMarker = L.marker([pt.geometry.coordinates[1], pt.geometry.coordinates[0]], {icon: iconDiv }).bindPopup(body, { maxWidth: 400 }).addTo(map);

        // Add event handlers
        newMarker.addEventListener('popupopen', () => {
            if (pt.can_delete) {
                const deleteButton = document.getElementById(`delete-btn-${ptId}`);
                deleteButton.addEventListener('click', (e) => deleteExistingPoint(ptId, newMarker));
            } else {
                const upVoteButton = document.getElementById(`upvote-btn-${ptId}`);
                const downVoteButton = document.getElementById(`downvote-btn-${ptId}`);

                if (isVerified) {
                    upVoteButton.addEventListener('click', (e) => submitPointVote(ptId, "upvote"));
                    downVoteButton.addEventListener('click', (e) => submitPointVote(ptId, "downvote"));
                } else {
                    upVoteButton.disabled = true
                    downVoteButton.disabled = true
                    downVoteButton.insertAdjacentElement('afterend', verificationRequiredNote("rating points"));
                }
            }
        });
    });
}

function divIconFor(emoji, opacity) {
    return L.divIcon({ html: `<p style="font-size: 250%; margin-left: -75%; margin-top: -75%; opacity: ${opacity};">${emoji}</p>` });
}

function opacityForAge(createdAt) {
    var millisecondsOld = new Date() - createdAt;

    // Fully visible for first 3 hours.
    if (millisecondsOld < 3 * 3600000) {
        return 1.0;
    // Lowest visibility of 0.1 if older than 12 hours (shouldn't ever reach
    // this case because we only return 12 hours old points).
    } else if (millisecondsOld > 12 * 3600000) {
        return 0.1;
    // Fade from 100% to 10% from 3 hours to 12 hours.
    } else {
        return millisecondsOld * (-1 / 36000000) + 1.3
    } 
}

var icon = "📍";
var placeholderPoint;
function createPoint(event) {
    const lat = event.latlng.lat;
    const lng = event.latlng.lng;
    placeholderPoint = L.marker([lat, lng], {icon: divIconFor(icon) }).addTo(map);

    // Present the popup box.
    var iconGrpId = `icon-grp_${lat}_${lng}`;
    var popup = L.popup({
        maxWidth: 300
    }).setLatLng([lat, lng]).setContent(`
        <form>
            <div class="form-group">
                <textarea id="msg-body" class="form-control" id="message" rows="3"></textarea>
            </div>
            <div id="a-${iconGrpId}" class="btn-group mr-2">
                <button type="button" class="btn btn-light">📍</button>
                <button type="button" class="btn btn-light">🚔</button>
                <button type="button" class="btn btn-light">☁️</button>
                <button type="button" class="btn btn-light">🚧</button>
                <button type="button" class="btn btn-light">🚰</button>
            </div>
            <div id="b-${iconGrpId}" class="btn-group mr-2">
                <button type="button" class="btn btn-light">📦</button>
                <button type="button" class="btn btn-light">🍕</button>
                <button type="button" class="btn btn-light">🔋</button>
                <button type="button" class="btn btn-light">🚽</button>
                <input type="text" class="form-control" placeholder="..." style="width: 3rem;" maxlength="1">
            </div>
            <button id="submit-btn" type="submit" class="btn btn-primary">Add</button>
        </form>
    `).openOn(map);

    for (var row of ["a", "b"]){
        const iconGrp = document.getElementById(`${row}-${iconGrpId}`);
        for (var i = 0; i < iconGrp.childElementCount; i++) {
            const elem = iconGrp.children[i];
            elem.addEventListener('click', (e) => {
                var prevIcon = icon;
                icon = elem.innerText || elem.value || prevIcon || "📍";
                changeHighlightedIcon(iconGrpId, i, row);

                // Redraw the placeholder with the correct icon.
                map.removeLayer(placeholderPoint);
                placeholderPoint = L.marker([lat, lng], {icon: divIconFor(icon) }).addTo(map);
            });

            elem.addEventListener('keyup', (e) => {
                var prevIcon = icon;
                icon = elem.innerText || elem.value || prevIcon || "📍";
                changeHighlightedIcon(iconGrpId, i, row);

                // Redraw the placeholder with the correct icon.
                map.removeLayer(placeholderPoint);
                placeholderPoint = L.marker([lat, lng], {icon: divIconFor(icon) }).addTo(map);
            });
        }
    }

    // POST to the API when the submit button is pressed.
    var submitBtn = document.getElementById('submit-btn');

    if (isVerified) {
        submitBtn.addEventListener('click', (e) => {
            e.preventDefault();
            submitBtn.disabled = true;

            var msg = document.getElementById('msg-body').value;
            postNewPoint(event.latlng, msg, icon)

            // Close the popup and remove the temporary point we added.
            map.removeLayer(popup);
            map.removeLayer(placeholderPoint);
        });
    } else {
        submitBtn.disabled = true
        submitBtn.insertAdjacentElement('afterend', verificationRequiredNote("adding points"));
    }

    // Remove temporary point we added if we close the box.
    popup._closeButton.addEventListener('click', (e) => {
        map.removeLayer(placeholderPoint);
    });
}

function verificationRequiredNote(action) {
    const note = document.createElement('p');
    note.innerHTML = `<a href="#">Verify your email before ${action}</a>`;
    note.addEventListener('click', (e) => {
        document.getElementById("verify-msg").hidden = false;
    });

    return note;
}

function changeHighlightedIcon(groupIdPrefix, idx, rowId) {
    for (var row of ["a", "b"]){
        const group = document.getElementById(`${row}-${groupIdPrefix}`);
        for (var i = 0; i < group.childElementCount; i++) {
            const elem = group.children[i];
            if (elem.type == "button") {
                if (i === idx && row === rowId) {
                    elem.classList = "btn btn-light active"    
                } else {
                    elem.classList = "btn btn-light"
                }
            }
        }
    }
}

function postNewPoint(coords, msg, icon) {
    var httpRequest = new XMLHttpRequest();

    httpRequest.onreadystatechange = () => {
        if (httpRequest.readyState === XMLHttpRequest.DONE) {
            if (httpRequest.status === 200) {
                var resp = JSON.parse(httpRequest.responseText);
                addNewPointsToMap(resp);

                // Set user is verified option here since the front-end may
                // be out of sync with the back-end but if the back-end
                // accepted the request then we know the user is verified.
                setUserIsVerified();
            } else {
                console.log("There was a problem with the request.");
            }
        }
    };

    var body = { coords: coords, message: msg, icon: icon };

    httpRequest.open('POST', '/api/v1/point');
    httpRequest.send(JSON.stringify(body));
}


function deleteExistingPoint(id, marker) {
    var httpRequest = new XMLHttpRequest();

    httpRequest.onreadystatechange = () => {
        if (httpRequest.readyState === XMLHttpRequest.DONE) {
            if (httpRequest.status === 200) {
                // Remove the point from the map on the front-end.
                map.removeLayer(marker);

                // Set user is verified option here since the front-end may
                // be out of sync with the back-end but if the back-end
                // accepted the request then we know the user is verified.
                setUserIsVerified();
            } else {
                console.log("There was a problem with the request.");
            }
        }
    };

    var body = { point_id: Number(id) };

    httpRequest.open('POST', '/api/v1/delete');
    httpRequest.send(JSON.stringify(body));
}

function submitPointVote(id, endpoint) {
    var httpRequest = new XMLHttpRequest();

    httpRequest.onreadystatechange = () => {
        if (httpRequest.readyState === XMLHttpRequest.DONE) {
            if (httpRequest.status === 200) {
                // Set user is verified option here since the front-end may
                // be out of sync with the back-end but if the back-end
                // accepted the request then we know the user is verified.
                setUserIsVerified();
            } else {
                console.log("There was a problem with the request.");
            }
        }
    };

    var body = { point_id: Number(id) };

    httpRequest.open('POST', `/api/v1/${endpoint}`);
    httpRequest.send(JSON.stringify(body));
}
