// ==UserScript==
// @name         Purify Jandan
// @namespace    http://tampermonkey.net/
// @version      2025-12-13
// @description  Purify Jandan by blocking certain users
// @author       maplestoria
// @match        https://jandan.net/*
// @icon         https://www.google.com/s2/favicons?sz=64&domain=jandan.net
// @grant        GM_setValue
// @grant        GM_getValue
// @grant        GM_getResourceText
// @grant        GM_log
// ==/UserScript==

(function () {
    'use strict';
    const lastUpdateTimeKey = "purifyjandan:lastFetchTime";
    const updateInterval = 24 * 60 * 60 * 1000; // 24 hours
    const lastUpdateTime = GM_getValue(lastUpdateTimeKey, null);
    GM_log("Purify Jandan: Last update time:", new Date(lastUpdateTime).toLocaleString());

    let blockedNickNames = localStorage.getItem("jandan:blockNickStore");
    let jso = blockedNickNames ? JSON.parse(blockedNickNames) : { blockedUsers: {} };
    if (!jso.blockedUsers || typeof jso.blockedUsers !== 'object') {
        jso.blockedUsers = {};
    }

    if (!lastUpdateTime
        || (Date.now() - lastUpdateTime) > updateInterval
        || Object.keys(jso.blockedUsers).length === 0) {
        GM_log("Purify Jandan: Fetching updated blocked users list...");

        const blockedUsers = "https://raw.githubusercontent.com/maplestoria/PurifyJandan/refs/heads/main/blocked_users.txt";

        GM_xmlhttpRequest({
            method: "GET",
            url: blockedUsers,
            nocache: true,
            timeout: 10000,
            onload: function (resp) {
                resp.responseText.split("\n")
                    .map(name => name.trim())
                    .filter(name => name.length > 0)
                    .forEach(name => {
                        if (!jso.blockedUsers[name]) {
                            GM_log("Purify Jandan: Blocking user:", name);
                            jso.blockedUsers[name] = true;
                            localStorage.setItem("jandan:blockNickStore", JSON.stringify(jso));
                        }
                    });
                GM_setValue(lastUpdateTimeKey, Date.now());
                localStorage.setItem("jandan:blockNickStore", JSON.stringify(jso));
                GM_log("Purify Jandan: Blocked users list updated.");
            },
            onerror: function (error) {
                GM_log('Error fetching blocked users list:' + error);
            },
            onabort : function () {
                GM_log('Request for blocked users list was aborted.');
            },
            ontimeout : function () {
                GM_log('Request for blocked users list timed out.');
            }
        });
    } else {
        GM_log("Purify Jandan: No update needed at this time.");
    }
})();