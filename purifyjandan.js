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
// @connect      cdn.jsdelivr.net
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

        const blockedUsers = "https://cdn.jsdelivr.net/gh/maplestoria/PurifyJandan@refs/heads/main/blocked_users.json";

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

    // 首页"热榜"屏蔽
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
                            blockedDiv.style.paddingBottom = "5px";

                            const unblockLink = document.createElement("a");
                            unblockLink.href = "javascript:;";
                            unblockLink.style.textDecoration = "none";
                            unblockLink.style.color = "#666";
                            unblockLink.innerText = '「手贱一下」';
                            unblockLink.style.fontSize = "12px";
                            blockedDiv.appendChild(unblockLink);

                            const savedChildren1 = item.querySelector("div.hot-title");
                            const savedChildren2 = item.querySelector("div.hot-content");
                            const savedChildren3 = item.querySelector("div.hot-vote");
                            savedChildren1.style.visibility = "hidden";
                            savedChildren1.style.height = "0";
                            savedChildren1.style.padding = "0";
                            savedChildren1.style.margin = "0";

                            savedChildren2.style.visibility = "hidden";
                            savedChildren2.style.height = "0";
                            savedChildren2.style.padding = "0";

                            savedChildren3.style.visibility = "hidden";
                            savedChildren3.style.height = "0";

                            unblockLink.addEventListener("click", function () {
                                savedChildren1.style.visibility = "visible";
                                savedChildren1.style.height = "auto";
                                savedChildren1.style.padding = "5px 20px 5px 12px";
                                savedChildren1.style.margin = "0 -12px 10px";

                                savedChildren2.style.visibility = "visible";
                                savedChildren2.style.height = "auto";
                                savedChildren2.style.padding = "0 0 30px 0";

                                savedChildren3.style.visibility = "visible";
                                savedChildren3.style.height = "auto";
                                item.style.margin = "0";
                                item.style.borderTop = "unset";
                                blockedDiv.remove();
                            });

                            item.appendChild(blockedDiv);
                            item.style.borderTop = "1px solid #e5e5e5";
                            item.style.margin = "0 -12px";
                        }
                    }
                    for (let i = 0; i < hotItems.length; i++) {
                        const item = hotItems[i];
                        const blockedDiv = item.querySelector("div.comment-block")
                        if (blockedDiv && i + 1 < hotItems.length) {
                            const nextItem = hotItems[i + 1];
                            const commentBlock = nextItem.querySelector("div.comment-block");
                            if (!commentBlock) {
                                blockedDiv.style.borderBottom = "1px solid #e5e5e5";
                            }
                        }
                    }
                }
            });
        }
    }
    // "热榜"页面屏蔽
    else if (window.location.pathname === '/top') {

        const targetNode = document.querySelector("div.post.p-0")
        const observerOptions = {
            childList: true,
            attributes: false,
            subtree: true
        }

        const observer = new MutationObserver(callback);
        observer.observe(targetNode, observerOptions);

        function callback(mutationList, observer) {
            mutationList.forEach((mutation) => {
                if (mutation.type === 'childList' && mutation.addedNodes.length === 0) {
                    let target = mutation.target;
                    if (target?.children.length > 0) {
                        for (let item of target.children) {
                            if (item.className === "comment-row p-2") {
                                const author = item.querySelector("span.author-anonymous, span.author-logged")?.innerText;
                                if (blockNickStore.blockedUsers[author] === true) {
                                    const blockedDiv = document.createElement("div");
                                    blockedDiv.className = "comment-block";
                                    blockedDiv.innerText = " 已屏蔽内容 ";
                                    blockedDiv.style.fontSize = "12px";
                                    blockedDiv.style.fontWeight = "400";
                                    blockedDiv.style.color = "#bbb";
                                    blockedDiv.style.textAlign = "center";
                                    blockedDiv.style.margin = "0 -12px";

                                    const unblockLink = document.createElement("a");
                                    unblockLink.href = "javascript:;";
                                    unblockLink.style.textDecoration = "none";
                                    unblockLink.style.color = "#666";
                                    unblockLink.innerText = '「手贱一下」';
                                    unblockLink.style.fontSize = "12px";
                                    blockedDiv.appendChild(unblockLink);
                                    item.appendChild(blockedDiv);

                                    const savedChildren1 = item.querySelector("div.comment-meta");
                                    const savedChildren2 = item.querySelector("div.comment-content");
                                    const savedChildren3 = item.querySelector("div.comment-func");
                                    savedChildren1.style.visibility = "hidden";
                                    savedChildren1.style.height = "0";
                                    savedChildren1.style.padding = "0";

                                    savedChildren2.style.visibility = "hidden";
                                    savedChildren2.style.height = "0";
                                    savedChildren2.style.padding = "0";

                                    savedChildren3.style.visibility = "hidden";
                                    savedChildren3.style.height = "0";

                                    unblockLink.addEventListener("click", function () {
                                        savedChildren1.style.visibility = "visible";
                                        savedChildren1.style.height = "auto";
                                        savedChildren1.style.padding = "5px 10px";

                                        savedChildren2.style.visibility = "visible";
                                        savedChildren2.style.height = "auto";
                                        savedChildren2.style.padding = "10px";

                                        savedChildren3.style.visibility = "visible";
                                        savedChildren3.style.height = "auto";
                                        blockedDiv.remove();
                                    });
                                }
                            } else if (item.className === "google-auto-placed") {
                                item.remove();
                            }
                        }
                    }
                }
            });
        };
    }
    // "大吐槽"页面屏蔽
    else if (window.location.pathname === "/tucao") {
        const targetNode = document.querySelector("#main-warpper > div.container > div > main > div:nth-child(2) > div.post.p-0 > div:nth-child(2)")
        const observerOptions = {
            childList: true,
            attributes: false,
            subtree: false
        }

        const observer = new MutationObserver(callback);
        observer.observe(targetNode, observerOptions);

        function callback(mutationList, observer) {
            mutationList.forEach((mutation) => {
                if (mutation.type === 'childList' && mutation.addedNodes.length === 0) {
                    let target = mutation.target;
                    if (target?.children.length > 0) {
                        for (let item of target.children) {
                            if (item.className === "comment-row p-2") {
                                const author = item.querySelector("span.author-anonymous, span.author-logged")?.innerText;
                                if (blockNickStore.blockedUsers[author] === true) {
                                    const blockedDiv = document.createElement("div");
                                    blockedDiv.className = "comment-block";
                                    blockedDiv.innerText = " 已屏蔽内容 ";
                                    blockedDiv.style.fontSize = "12px";
                                    blockedDiv.style.fontWeight = "400";
                                    blockedDiv.style.color = "#bbb";
                                    blockedDiv.style.textAlign = "center";
                                    blockedDiv.style.margin = "0 -12px";

                                    const unblockLink = document.createElement("a");
                                    unblockLink.href = "javascript:;";
                                    unblockLink.style.textDecoration = "none";
                                    unblockLink.style.color = "#666";
                                    unblockLink.innerText = '「手贱一下」';
                                    unblockLink.style.fontSize = "12px";
                                    blockedDiv.appendChild(unblockLink);
                                    item.appendChild(blockedDiv);

                                    const savedChildren1 = item.querySelector("div.comment-meta");
                                    const savedChildren2 = item.querySelector("div.comment-content");
                                    const savedChildren3 = item.querySelector("div.comment-func");
                                    const savedChildren4 = item.querySelector("div.tucao-container.p-2");
                                    savedChildren1.style.visibility = "hidden";
                                    savedChildren1.style.height = "0";
                                    savedChildren1.style.padding = "0";

                                    savedChildren2.style.visibility = "hidden";
                                    savedChildren2.style.height = "0";
                                    savedChildren2.style.padding = "0";

                                    savedChildren3.style.visibility = "hidden";
                                    savedChildren3.style.height = "0";

                                    savedChildren4.style.visibility = "hidden";
                                    savedChildren4.style.height = "0";

                                    unblockLink.addEventListener("click", function () {
                                        savedChildren1.style.visibility = "visible";
                                        savedChildren1.style.height = "auto";
                                        savedChildren1.style.padding = "5px 10px";

                                        savedChildren2.style.visibility = "visible";
                                        savedChildren2.style.height = "auto";
                                        savedChildren2.style.padding = "10px";

                                        savedChildren3.style.visibility = "visible";
                                        savedChildren3.style.height = "auto";

                                        savedChildren4.style.visibility = "visible";
                                        savedChildren4.style.height = "auto";
                                        blockedDiv.remove();
                                    });
                                }
                            } else if (item.className === "google-auto-placed") {
                                item.remove();
                            }
                        }
                    }
                }
            });
        }
    }
})();