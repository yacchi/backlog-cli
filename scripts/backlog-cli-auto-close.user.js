// ==UserScript==
// @name         Backlog CLI Auto Close
// @name:ja      Backlog CLI 自動クローズ
// @description  Enable auto-close for Backlog CLI authentication page
// @description:ja  Backlog CLI の認証完了ページを自動的に閉じます
// @version      1.2.0
// @author       yacchi
// @match        http://localhost:*/auth/start*
// @match        http://localhost:*/auth/setup*
// @match        http://localhost:*/auth/complete*
// @grant        window.close
// @grant        unsafeWindow
// @run-at       document-start
// @license      Apache-2.0
// @homepageURL  https://github.com/yacchi/backlog-cli
// @supportURL   https://github.com/yacchi/backlog-cli/issues
// ==/UserScript==

unsafeWindow.forceCloseTab = () => window.close();