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
    let blockNickStore = blockedNickNames ? JSON.parse(blockedNickNames) : { blockedUsers: {} };
    if (!blockNickStore.blockedUsers || typeof blockNickStore.blockedUsers !== 'object') {
        blockNickStore.blockedUsers = {};
    }

    let blockedIds = localStorage.getItem("jandan:blockIDStore");
    let blockIDStore = blockedIds ? JSON.parse(blockedIds) : { blockedUsers: {} };
    if (!blockIDStore.blockedUsers || typeof blockIDStore.blockedUsers !== 'object') {
        blockIDStore.blockedUsers = {};
    }

    if (!lastUpdateTime
        || (Date.now() - lastUpdateTime) > updateInterval
        || Object.keys(blockNickStore.blockedUsers).length === 0) {
        GM_log("Purify Jandan: Fetching updated blocked users list...");

        const blockedUsers = "https://raw.githubusercontent.com/maplestoria/PurifyJandan/refs/heads/main/blocked_users.json";

        GM_xmlhttpRequest({
            method: "GET",
            url: blockedUsers,
            nocache: true,
            timeout: 10000,
            onload: function (resp) {
                const blcoked = JSON.parse(resp.responseText);
                blcoked.nicknames.forEach(name => {
                    if (!blockNickStore.blockedUsers[name]) {
                        GM_log("Purify Jandan: Blocking user:", name);
                        blockNickStore.blockedUsers[name] = true;
                    }
                });
                blcoked.ids.forEach(id => {
                    if (!blockIDStore.blockedUsers[id]) {
                        GM_log("Purify Jandan: Blocking user ID:", id);
                        blockIDStore.blockedUsers[id] = true;
                    }
                });
                localStorage.setItem("jandan:blockIDStore", JSON.stringify(blockIDStore));
                localStorage.setItem("jandan:blockNickStore", JSON.stringify(blockNickStore));

                GM_setValue(lastUpdateTimeKey, Date.now());
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