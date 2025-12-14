// ==UserScript==
// @name         Purify Jandan
// @namespace    http://tampermonkey.net/
// @version      2025-12-13
// @description  Purify Jandan by blocking certain users
// @author       maplestoria
// @homepage     https://github.com/maplestoria/PurifyJandan
// @match        https://jandan.net/*
// @icon         https://www.google.com/s2/favicons?sz=64&domain=jandan.net
// @grant        GM_setValue
// @grant        GM_getValue
// @grant        GM_log
// @grant        GM_xmlhttpRequest
// @connect      raw.githubusercontent.com
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
            onabort: function () {
                GM_log('Request for blocked users list was aborted.');
            },
            ontimeout: function () {
                GM_log('Request for blocked users list timed out.');
            }
        });
    } else {
        GM_log("Purify Jandan: No update needed at this time.");
    }

    if (window.location.pathname === '/') {
        const targetNodes = document.querySelectorAll("div#list-hot, div#list-pic, div#list-ooxx, div#list-treehole")
        const observerOptions = {
            childList: true,
            attributes: false,
            subtree: true
        }

        const observer = new MutationObserver(callback);
        for (const node of targetNodes) {
            observer.observe(node, observerOptions);
        }

        function callback(mutationList, observer) {
            mutationList.forEach((mutation) => {
                if (mutation.type === 'childList' && mutation.addedNodes.length === 3) {
                    let hotItems = mutation.addedNodes[1].children
                    for (let item of hotItems) {
                        const title = item.querySelector("div.hot-title").innerText
                        const userNickName = title.substring(0, title.indexOf("@") - 1)
                        if (blockNickStore.blockedUsers[userNickName] === true) {
                            const blockedDiv = document.createElement("div");
                            blockedDiv.className = "comment-block";
                            blockedDiv.innerText = " 已屏蔽内容 ";
                            blockedDiv.style.fontSize = "12px";
                            blockedDiv.style.fontWeight = "400";
                            blockedDiv.style.color = "#bbb";
                            blockedDiv.style.textAlign = "center";
                            blockedDiv.style.padding = "5px 20px 5px 12px";
                            blockedDiv.style.margin = "0 -12px";
                            blockedDiv.style.borderTop = "1px solid #e5e5e5";

                            const unblockLink = document.createElement("a");
                            unblockLink.href = "javascript:;";
                            unblockLink.style.textDecoration = "none";
                            unblockLink.style.color = "#666";
                            unblockLink.innerText = '「手贱一下」';
                            unblockLink.style.fontSize = "12px";
                            blockedDiv.appendChild(unblockLink);
                            
                            const savedChildren1 = item.querySelector("div.hot-title").cloneNode(true);
                            const savedChildren2 = item.querySelector("div.hot-content").cloneNode(true);
                            const savedChildren3 = item.querySelector("div.hot-vote").cloneNode(true);
                            unblockLink.addEventListener("click", function () {
                                item.replaceChildren(savedChildren1, savedChildren2, savedChildren3);
                            });
                            
                            item.replaceChildren(blockedDiv);
                        }
                    }
                    for (let i = 0; i < hotItems.length; i++) {
                        const item = hotItems[i];
                        const blockedDiv = item.querySelector("div.comment-block")
                        if (blockedDiv) {
                            const nextItem = hotItems[i + 1];
                            const normalItem = nextItem.querySelector("div.hot-title");
                            if (normalItem) {
                                blockedDiv.style.borderBottom = "1px solid #e5e5e5";
                            }
                        }
                    }
                    observer.disconnect();
                }
            });
        }
    }
})();