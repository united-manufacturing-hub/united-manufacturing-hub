define(["@grafana/data","@grafana/runtime","@grafana/ui","lodash","react"], function(__WEBPACK_EXTERNAL_MODULE__grafana_data__, __WEBPACK_EXTERNAL_MODULE__grafana_runtime__, __WEBPACK_EXTERNAL_MODULE__grafana_ui__, __WEBPACK_EXTERNAL_MODULE_lodash__, __WEBPACK_EXTERNAL_MODULE_react__) { return /******/ (function(modules) { // webpackBootstrap
/******/ 	// The module cache
/******/ 	var installedModules = {};
/******/
/******/ 	// The require function
/******/ 	function __webpack_require__(moduleId) {
/******/
/******/ 		// Check if module is in cache
/******/ 		if(installedModules[moduleId]) {
/******/ 			return installedModules[moduleId].exports;
/******/ 		}
/******/ 		// Create a new module (and put it into the cache)
/******/ 		var module = installedModules[moduleId] = {
/******/ 			i: moduleId,
/******/ 			l: false,
/******/ 			exports: {}
/******/ 		};
/******/
/******/ 		// Execute the module function
/******/ 		modules[moduleId].call(module.exports, module, module.exports, __webpack_require__);
/******/
/******/ 		// Flag the module as loaded
/******/ 		module.l = true;
/******/
/******/ 		// Return the exports of the module
/******/ 		return module.exports;
/******/ 	}
/******/
/******/
/******/ 	// expose the modules object (__webpack_modules__)
/******/ 	__webpack_require__.m = modules;
/******/
/******/ 	// expose the module cache
/******/ 	__webpack_require__.c = installedModules;
/******/
/******/ 	// define getter function for harmony exports
/******/ 	__webpack_require__.d = function(exports, name, getter) {
/******/ 		if(!__webpack_require__.o(exports, name)) {
/******/ 			Object.defineProperty(exports, name, { enumerable: true, get: getter });
/******/ 		}
/******/ 	};
/******/
/******/ 	// define __esModule on exports
/******/ 	__webpack_require__.r = function(exports) {
/******/ 		if(typeof Symbol !== 'undefined' && Symbol.toStringTag) {
/******/ 			Object.defineProperty(exports, Symbol.toStringTag, { value: 'Module' });
/******/ 		}
/******/ 		Object.defineProperty(exports, '__esModule', { value: true });
/******/ 	};
/******/
/******/ 	// create a fake namespace object
/******/ 	// mode & 1: value is a module id, require it
/******/ 	// mode & 2: merge all properties of value into the ns
/******/ 	// mode & 4: return value when already ns object
/******/ 	// mode & 8|1: behave like require
/******/ 	__webpack_require__.t = function(value, mode) {
/******/ 		if(mode & 1) value = __webpack_require__(value);
/******/ 		if(mode & 8) return value;
/******/ 		if((mode & 4) && typeof value === 'object' && value && value.__esModule) return value;
/******/ 		var ns = Object.create(null);
/******/ 		__webpack_require__.r(ns);
/******/ 		Object.defineProperty(ns, 'default', { enumerable: true, value: value });
/******/ 		if(mode & 2 && typeof value != 'string') for(var key in value) __webpack_require__.d(ns, key, function(key) { return value[key]; }.bind(null, key));
/******/ 		return ns;
/******/ 	};
/******/
/******/ 	// getDefaultExport function for compatibility with non-harmony modules
/******/ 	__webpack_require__.n = function(module) {
/******/ 		var getter = module && module.__esModule ?
/******/ 			function getDefault() { return module['default']; } :
/******/ 			function getModuleExports() { return module; };
/******/ 		__webpack_require__.d(getter, 'a', getter);
/******/ 		return getter;
/******/ 	};
/******/
/******/ 	// Object.prototype.hasOwnProperty.call
/******/ 	__webpack_require__.o = function(object, property) { return Object.prototype.hasOwnProperty.call(object, property); };
/******/
/******/ 	// __webpack_public_path__
/******/ 	__webpack_require__.p = "/";
/******/
/******/
/******/ 	// Load entry module and return exports
/******/ 	return __webpack_require__(__webpack_require__.s = "./module.ts");
/******/ })
/************************************************************************/
/******/ ({

/***/ "../node_modules/tslib/tslib.es6.js":
/*!******************************************!*\
  !*** ../node_modules/tslib/tslib.es6.js ***!
  \******************************************/
/*! exports provided: __extends, __assign, __rest, __decorate, __param, __metadata, __awaiter, __generator, __createBinding, __exportStar, __values, __read, __spread, __spreadArrays, __await, __asyncGenerator, __asyncDelegator, __asyncValues, __makeTemplateObject, __importStar, __importDefault, __classPrivateFieldGet, __classPrivateFieldSet */
/***/ (function(module, __webpack_exports__, __webpack_require__) {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__extends", function() { return __extends; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__assign", function() { return __assign; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__rest", function() { return __rest; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__decorate", function() { return __decorate; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__param", function() { return __param; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__metadata", function() { return __metadata; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__awaiter", function() { return __awaiter; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__generator", function() { return __generator; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__createBinding", function() { return __createBinding; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__exportStar", function() { return __exportStar; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__values", function() { return __values; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__read", function() { return __read; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__spread", function() { return __spread; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__spreadArrays", function() { return __spreadArrays; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__await", function() { return __await; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__asyncGenerator", function() { return __asyncGenerator; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__asyncDelegator", function() { return __asyncDelegator; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__asyncValues", function() { return __asyncValues; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__makeTemplateObject", function() { return __makeTemplateObject; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__importStar", function() { return __importStar; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__importDefault", function() { return __importDefault; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__classPrivateFieldGet", function() { return __classPrivateFieldGet; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "__classPrivateFieldSet", function() { return __classPrivateFieldSet; });
/*! *****************************************************************************
Copyright (c) Microsoft Corporation.

Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY
AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM
LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR
OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR
PERFORMANCE OF THIS SOFTWARE.
***************************************************************************** */
/* global Reflect, Promise */

var extendStatics = function(d, b) {
    extendStatics = Object.setPrototypeOf ||
        ({ __proto__: [] } instanceof Array && function (d, b) { d.__proto__ = b; }) ||
        function (d, b) { for (var p in b) if (b.hasOwnProperty(p)) d[p] = b[p]; };
    return extendStatics(d, b);
};

function __extends(d, b) {
    extendStatics(d, b);
    function __() { this.constructor = d; }
    d.prototype = b === null ? Object.create(b) : (__.prototype = b.prototype, new __());
}

var __assign = function() {
    __assign = Object.assign || function __assign(t) {
        for (var s, i = 1, n = arguments.length; i < n; i++) {
            s = arguments[i];
            for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p)) t[p] = s[p];
        }
        return t;
    }
    return __assign.apply(this, arguments);
}

function __rest(s, e) {
    var t = {};
    for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p) && e.indexOf(p) < 0)
        t[p] = s[p];
    if (s != null && typeof Object.getOwnPropertySymbols === "function")
        for (var i = 0, p = Object.getOwnPropertySymbols(s); i < p.length; i++) {
            if (e.indexOf(p[i]) < 0 && Object.prototype.propertyIsEnumerable.call(s, p[i]))
                t[p[i]] = s[p[i]];
        }
    return t;
}

function __decorate(decorators, target, key, desc) {
    var c = arguments.length, r = c < 3 ? target : desc === null ? desc = Object.getOwnPropertyDescriptor(target, key) : desc, d;
    if (typeof Reflect === "object" && typeof Reflect.decorate === "function") r = Reflect.decorate(decorators, target, key, desc);
    else for (var i = decorators.length - 1; i >= 0; i--) if (d = decorators[i]) r = (c < 3 ? d(r) : c > 3 ? d(target, key, r) : d(target, key)) || r;
    return c > 3 && r && Object.defineProperty(target, key, r), r;
}

function __param(paramIndex, decorator) {
    return function (target, key) { decorator(target, key, paramIndex); }
}

function __metadata(metadataKey, metadataValue) {
    if (typeof Reflect === "object" && typeof Reflect.metadata === "function") return Reflect.metadata(metadataKey, metadataValue);
}

function __awaiter(thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
}

function __generator(thisArg, body) {
    var _ = { label: 0, sent: function() { if (t[0] & 1) throw t[1]; return t[1]; }, trys: [], ops: [] }, f, y, t, g;
    return g = { next: verb(0), "throw": verb(1), "return": verb(2) }, typeof Symbol === "function" && (g[Symbol.iterator] = function() { return this; }), g;
    function verb(n) { return function (v) { return step([n, v]); }; }
    function step(op) {
        if (f) throw new TypeError("Generator is already executing.");
        while (_) try {
            if (f = 1, y && (t = op[0] & 2 ? y["return"] : op[0] ? y["throw"] || ((t = y["return"]) && t.call(y), 0) : y.next) && !(t = t.call(y, op[1])).done) return t;
            if (y = 0, t) op = [op[0] & 2, t.value];
            switch (op[0]) {
                case 0: case 1: t = op; break;
                case 4: _.label++; return { value: op[1], done: false };
                case 5: _.label++; y = op[1]; op = [0]; continue;
                case 7: op = _.ops.pop(); _.trys.pop(); continue;
                default:
                    if (!(t = _.trys, t = t.length > 0 && t[t.length - 1]) && (op[0] === 6 || op[0] === 2)) { _ = 0; continue; }
                    if (op[0] === 3 && (!t || (op[1] > t[0] && op[1] < t[3]))) { _.label = op[1]; break; }
                    if (op[0] === 6 && _.label < t[1]) { _.label = t[1]; t = op; break; }
                    if (t && _.label < t[2]) { _.label = t[2]; _.ops.push(op); break; }
                    if (t[2]) _.ops.pop();
                    _.trys.pop(); continue;
            }
            op = body.call(thisArg, _);
        } catch (e) { op = [6, e]; y = 0; } finally { f = t = 0; }
        if (op[0] & 5) throw op[1]; return { value: op[0] ? op[1] : void 0, done: true };
    }
}

function __createBinding(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}

function __exportStar(m, exports) {
    for (var p in m) if (p !== "default" && !exports.hasOwnProperty(p)) exports[p] = m[p];
}

function __values(o) {
    var s = typeof Symbol === "function" && Symbol.iterator, m = s && o[s], i = 0;
    if (m) return m.call(o);
    if (o && typeof o.length === "number") return {
        next: function () {
            if (o && i >= o.length) o = void 0;
            return { value: o && o[i++], done: !o };
        }
    };
    throw new TypeError(s ? "Object is not iterable." : "Symbol.iterator is not defined.");
}

function __read(o, n) {
    var m = typeof Symbol === "function" && o[Symbol.iterator];
    if (!m) return o;
    var i = m.call(o), r, ar = [], e;
    try {
        while ((n === void 0 || n-- > 0) && !(r = i.next()).done) ar.push(r.value);
    }
    catch (error) { e = { error: error }; }
    finally {
        try {
            if (r && !r.done && (m = i["return"])) m.call(i);
        }
        finally { if (e) throw e.error; }
    }
    return ar;
}

function __spread() {
    for (var ar = [], i = 0; i < arguments.length; i++)
        ar = ar.concat(__read(arguments[i]));
    return ar;
}

function __spreadArrays() {
    for (var s = 0, i = 0, il = arguments.length; i < il; i++) s += arguments[i].length;
    for (var r = Array(s), k = 0, i = 0; i < il; i++)
        for (var a = arguments[i], j = 0, jl = a.length; j < jl; j++, k++)
            r[k] = a[j];
    return r;
};

function __await(v) {
    return this instanceof __await ? (this.v = v, this) : new __await(v);
}

function __asyncGenerator(thisArg, _arguments, generator) {
    if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
    var g = generator.apply(thisArg, _arguments || []), i, q = [];
    return i = {}, verb("next"), verb("throw"), verb("return"), i[Symbol.asyncIterator] = function () { return this; }, i;
    function verb(n) { if (g[n]) i[n] = function (v) { return new Promise(function (a, b) { q.push([n, v, a, b]) > 1 || resume(n, v); }); }; }
    function resume(n, v) { try { step(g[n](v)); } catch (e) { settle(q[0][3], e); } }
    function step(r) { r.value instanceof __await ? Promise.resolve(r.value.v).then(fulfill, reject) : settle(q[0][2], r); }
    function fulfill(value) { resume("next", value); }
    function reject(value) { resume("throw", value); }
    function settle(f, v) { if (f(v), q.shift(), q.length) resume(q[0][0], q[0][1]); }
}

function __asyncDelegator(o) {
    var i, p;
    return i = {}, verb("next"), verb("throw", function (e) { throw e; }), verb("return"), i[Symbol.iterator] = function () { return this; }, i;
    function verb(n, f) { i[n] = o[n] ? function (v) { return (p = !p) ? { value: __await(o[n](v)), done: n === "return" } : f ? f(v) : v; } : f; }
}

function __asyncValues(o) {
    if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
    var m = o[Symbol.asyncIterator], i;
    return m ? m.call(o) : (o = typeof __values === "function" ? __values(o) : o[Symbol.iterator](), i = {}, verb("next"), verb("throw"), verb("return"), i[Symbol.asyncIterator] = function () { return this; }, i);
    function verb(n) { i[n] = o[n] && function (v) { return new Promise(function (resolve, reject) { v = o[n](v), settle(resolve, reject, v.done, v.value); }); }; }
    function settle(resolve, reject, d, v) { Promise.resolve(v).then(function(v) { resolve({ value: v, done: d }); }, reject); }
}

function __makeTemplateObject(cooked, raw) {
    if (Object.defineProperty) { Object.defineProperty(cooked, "raw", { value: raw }); } else { cooked.raw = raw; }
    return cooked;
};

function __importStar(mod) {
    if (mod && mod.__esModule) return mod;
    var result = {};
    if (mod != null) for (var k in mod) if (Object.hasOwnProperty.call(mod, k)) result[k] = mod[k];
    result.default = mod;
    return result;
}

function __importDefault(mod) {
    return (mod && mod.__esModule) ? mod : { default: mod };
}

function __classPrivateFieldGet(receiver, privateMap) {
    if (!privateMap.has(receiver)) {
        throw new TypeError("attempted to get private field on non-instance");
    }
    return privateMap.get(receiver);
}

function __classPrivateFieldSet(receiver, privateMap, value) {
    if (!privateMap.has(receiver)) {
        throw new TypeError("attempted to set private field on non-instance");
    }
    privateMap.set(receiver, value);
    return value;
}


/***/ }),

/***/ "./ConfigEditor.tsx":
/*!**************************!*\
  !*** ./ConfigEditor.tsx ***!
  \**************************/
/*! exports provided: ConfigEditor */
/***/ (function(module, __webpack_exports__, __webpack_require__) {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "ConfigEditor", function() { return ConfigEditor; });
/* harmony import */ var tslib__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(/*! tslib */ "../node_modules/tslib/tslib.es6.js");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(/*! react */ "react");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _grafana_ui__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(/*! @grafana/ui */ "@grafana/ui");
/* harmony import */ var _grafana_ui__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__);




var ConfigEditor =
/** @class */
function (_super) {
  Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__extends"])(ConfigEditor, _super);

  function ConfigEditor() {
    var _this = _super !== null && _super.apply(this, arguments) || this;

    _this.onCustomerChange = function (event) {
      var _a = _this.props,
          onOptionsChange = _a.onOptionsChange,
          options = _a.options;

      var jsonData = Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options.jsonData), {
        customerId: event.target.value
      });

      onOptionsChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options), {
        jsonData: jsonData
      }));
    };

    _this.onServerChange = function (event) {
      var _a = _this.props,
          onOptionsChange = _a.onOptionsChange,
          options = _a.options;

      var jsonData = Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options.jsonData), {
        serverURL: event.target.value
      });

      onOptionsChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options), {
        jsonData: jsonData
      }));
    };

    _this.onBrokerURIChange = function (event) {
      var _a = _this.props,
          onOptionsChange = _a.onOptionsChange,
          options = _a.options;

      var jsonData = Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options.jsonData), {
        brokerURI: event.target.value
      });

      onOptionsChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options), {
        jsonData: jsonData
      }));
    };

    _this.onHistoricalEnabledChange = function (event) {
      var _a = _this.props,
          onOptionsChange = _a.onOptionsChange,
          options = _a.options;

      var jsonData = Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options.jsonData), {
        historicalEnabled: !options.jsonData.historicalEnabled
      });

      onOptionsChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options), {
        jsonData: jsonData
      }));
    };

    _this.onRealtimeEnabledChange = function (event) {
      var _a = _this.props,
          onOptionsChange = _a.onOptionsChange,
          options = _a.options;

      var jsonData = Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options.jsonData), {
        realtimeEnabled: !options.jsonData.realtimeEnabled
      });

      onOptionsChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options), {
        jsonData: jsonData
      }));
    };

    _this.onSecureConnectionMQTTChange = function (event) {
      var _a = _this.props,
          onOptionsChange = _a.onOptionsChange,
          options = _a.options;

      var jsonData = Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options.jsonData), {
        secureConnectionMQTT: !options.jsonData.secureConnectionMQTT
      });

      onOptionsChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options), {
        jsonData: jsonData
      }));
    }; // Secure field (only sent to the backend)


    _this.onAPIKeyChange = function (event) {
      var _a = _this.props,
          onOptionsChange = _a.onOptionsChange,
          options = _a.options;
      onOptionsChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options), {
        secureJsonData: Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options.secureJsonData), {
          apiKey: event.target.value
        })
      }));
    }; // Secure field (only sent to the backend)


    _this.onCACertChange = function (event) {
      var _a = _this.props,
          onOptionsChange = _a.onOptionsChange,
          options = _a.options;
      var value = event.target;
      onOptionsChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options), {
        secureJsonData: Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options.secureJsonData), {
          CACert: value.value
        })
      }));
    }; // Secure field (only sent to the backend)


    _this.onPrivkeyChange = function (event) {
      var _a = _this.props,
          onOptionsChange = _a.onOptionsChange,
          options = _a.options;
      var value = event.target;
      onOptionsChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options), {
        secureJsonData: Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, options.secureJsonData), {
          privkey: value.value
        })
      }));
    };

    return _this;
  }

  ConfigEditor.prototype.render = function () {
    var options = this.props.options;
    var jsonData = options.jsonData;
    var secureJsonData = options.secureJsonData || {};
    return react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", null, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", {
      className: "gf-form-group"
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("h3", {
      className: "page-heading"
    }, "Historical data"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineFieldRow"], null, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineField"], {
      label: "Enabled",
      labelWidth: 20
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Switch"], {
      css: "",
      width: 40,
      value: jsonData.historicalEnabled,
      onChange: this.onHistoricalEnabledChange
    }))), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineFieldRow"], {
      style: {
        display: jsonData.historicalEnabled ? "block" : "none"
      }
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineField"], {
      label: "API path",
      labelWidth: 20,
      tooltip: "The URL to the factoryinsight backend including a leading http or https."
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Input"], {
      css: "",
      width: 40,
      onChange: this.onServerChange,
      value: jsonData.serverURL || '',
      placeholder: "http://factoryinsight.factoryinsight"
    }))), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineFieldRow"], {
      style: {
        display: jsonData.historicalEnabled ? "block" : "none"
      }
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineField"], {
      label: "API token",
      labelWidth: 20,
      tooltip: "The API token for the backend. This can be created using Basic Auth e.g., in Postman"
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Input"], {
      css: "",
      width: 40,
      onChange: this.onAPIKeyChange,
      value: secureJsonData.apiKey || '',
      placeholder: "Basic xxxxxxxx"
    })))), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", {
      className: "gf-form-group"
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("h3", {
      className: "page-heading"
    }, "Real-time data"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineFieldRow"], null, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineField"], {
      label: "Enabled",
      labelWidth: 20
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Switch"], {
      css: "",
      width: 40,
      value: jsonData.realtimeEnabled,
      onChange: this.onRealtimeEnabledChange
    }))), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineFieldRow"], {
      style: {
        display: jsonData.realtimeEnabled ? "block" : "none"
      }
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineField"], {
      label: "Broker URI",
      labelWidth: 20,
      tooltip: "The URI to access the MQTT broker including a leading mqtt:// (unencrypted) or mqtts:// (encrypted)"
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Input"], {
      css: "",
      width: 40,
      onChange: this.onBrokerURIChange,
      value: jsonData.brokerURI || '',
      placeholder: "mqtts://vernemq.vernemq"
    }))), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineFieldRow"], {
      style: {
        display: jsonData.realtimeEnabled ? "block" : "none"
      }
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineField"], {
      label: "Secure connection",
      labelWidth: 20
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Switch"], {
      css: "",
      width: 40,
      value: jsonData.secureConnectionMQTT,
      onChange: this.onSecureConnectionMQTTChange
    }))), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineFieldRow"], {
      style: {
        display: jsonData.realtimeEnabled && jsonData.secureConnectionMQTT ? "block" : "none"
      }
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineField"], {
      label: "CA cert (PEM format)",
      labelWidth: 20
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["TextArea"], {
      css: "",
      width: 80,
      onChange: this.onCACertChange,
      value: secureJsonData.CACert || '',
      placeholder: "123"
    }))), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineFieldRow"], {
      style: {
        display: jsonData.realtimeEnabled && jsonData.secureConnectionMQTT ? "block" : "none"
      }
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["InlineField"], {
      label: "Privkey (PEM format)",
      labelWidth: 20
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["TextArea"], {
      css: "",
      width: 80,
      onChange: this.onPrivkeyChange,
      value: secureJsonData.privkey || '',
      placeholder: "123"
    })))));
  };

  return ConfigEditor;
}(react__WEBPACK_IMPORTED_MODULE_1__["PureComponent"]);



/***/ }),

/***/ "./QueryEditor.tsx":
/*!*************************!*\
  !*** ./QueryEditor.tsx ***!
  \*************************/
/*! exports provided: QueryEditor */
/***/ (function(module, __webpack_exports__, __webpack_require__) {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "QueryEditor", function() { return QueryEditor; });
/* harmony import */ var tslib__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(/*! tslib */ "../node_modules/tslib/tslib.es6.js");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(/*! react */ "react");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _grafana_ui__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(/*! @grafana/ui */ "@grafana/ui");
/* harmony import */ var _grafana_ui__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__);
/* harmony import */ var _types__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(/*! ./types */ "./types.ts");
/* harmony import */ var components_values_AggregatedStates__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(/*! components/values/AggregatedStates */ "./components/values/AggregatedStates.tsx");
/* harmony import */ var lodash__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(/*! lodash */ "lodash");
/* harmony import */ var lodash__WEBPACK_IMPORTED_MODULE_5___default = /*#__PURE__*/__webpack_require__.n(lodash__WEBPACK_IMPORTED_MODULE_5__);







var QueryEditor =
/** @class */
function (_super) {
  Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__extends"])(QueryEditor, _super);

  function QueryEditor(props) {
    var _this = _super.call(this, props) || this; // Event handler for the location dropdown


    _this.onLocationChange = function (event) {
      // Prevent change if the currently selected location was reclicked
      if (event.value === _this.state.selectedLocation.index) {
        return;
      } // Update the state with the selected location. When the location
      // changes, everything else resets.


      _this.setState({
        selectedLocation: {
          label: event.label || '',
          index: event.value || 0
        },
        selectedAsset: {
          label: '',
          index: 0
        },
        selectedValue: {
          label: '',
          index: 0
        },
        parameterString: ''
      }); // Update the location-related assets from the server

      /* this.props.datasource.getAssets(event.label || '', this.setAssetsOptions); */

    }; // This assumes that the server resturns an array of strings:
    // ['asset1', 'asset2', ...]


    _this.setAssetsOptions = function (assetArray) {
      // Get the last asset
      var assetIndex = _this.state.selectedAsset.index; // Update state with new asset options

      var newAssetLabel = assetArray[assetIndex];
      var newAssetOptions = assetArray.map(function (asset, index) {
        return {
          label: asset,
          value: index
        };
      });

      _this.setState({
        selectedAsset: {
          label: newAssetLabel,
          index: assetIndex
        },
        assetOptions: newAssetOptions
      }); // Get the location-asset-related values from the server

      /* this.props.datasource.getValues(this.state.selectedLocation.label, newAssetLabel, this.setValueOptions); */

    }; // Event handler for the asset dropdown


    _this.onAssetChange = function (event) {
      // Prevent change if the currently selected asset was reclicked
      if (event.value === _this.state.selectedAsset.index) {
        return;
      } // Update the state with the selected asset. If the asset changes
      // only the value and the parameter string are reset.


      _this.setState({
        selectedAsset: {
          label: event.label || '',
          index: event.value || 0
        },
        selectedValue: {
          label: '',
          index: 0
        },
        parameterString: ''
      }); // Get the location-asset-related values from the server

      /* this.props.datasource.getValues(this.state.selectedLocation.label, event.label || '', this.setValueOptions); */

    }; // Update the state with the selected asset


    _this.setValueOptions = function (valueArray) {
      // Get the last value
      var valueIndex = _this.state.selectedValue.index; // Update state with new value options

      var newValue = {
        label: valueArray[valueIndex] || '',
        index: valueIndex
      }; // Renaming value to metric inside this function because
      // the options object has a 'value' key.

      var newValueOptions = valueArray.map(function (metric, index) {
        return {
          label: metric,
          value: index
        };
      });

      _this.setState({
        selectedValue: newValue,
        valueOptions: newValueOptions
      }); // Update the default parameter string and update the chart


      _this.setParameters(newValue);
    }; // Handle value dropdown changes


    _this.onValueChange = function (event) {
      // Prevent change if the currently selected asset was reclicked
      if (event.value === _this.state.selectedValue.index) {
        return;
      } // Update the state with the selected value. If the value changes
      // only the paremeter string is reset


      var newValue = {
        label: event.label || '',
        index: event.value || 0
      };

      _this.setState({
        selectedValue: newValue,
        parameterString: ''
      }); // Get the location-asset-value-related parameters


      _this.setParameters(newValue);
    }; // TODO handle parameters properly with a data model or something


    _this.setParameters = function (value) {
      var newParameterString = _this.state.parameterString;

      if (value.label === 'aggregatedStates') {
        if (_this.state.parameterString === '') {
          newParameterString = Object(components_values_AggregatedStates__WEBPACK_IMPORTED_MODULE_4__["aggregatedStatesDefaultString"])();

          _this.setState({
            parameterString: newParameterString
          });
        }
      } // Get the location-asset-value-related datapoints from the server
      // and update the default parameter string


      _this.updateChart(value, newParameterString);
    }; // Handle parameters change


    _this.onParametersChange = function (parameterString) {
      // Update the state with the new parameters
      var newParameterString = parameterString;

      _this.setState({
        parameterString: newParameterString
      }); // Update chart


      _this.updateChart(_this.state.selectedValue, newParameterString);
    }; // Handle isCommand change


    _this.onIsCommandChange = function (event) {
      var _a = _this.props,
          onChange = _a.onChange,
          query = _a.query,
          onRunQuery = _a.onRunQuery; // Update the state with the new parameters

      _this.setState({
        isCommand: !_this.state.isCommand
      });

      console.log(_this.state.isCommand);
      onChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, query), {
        location: _this.state.selectedLocation,
        asset: _this.state.selectedAsset,
        value: _this.state.selectedValue,
        parameterString: _this.state.parameterString,
        labelsField: _this.state.labelsField,
        isCommand: _this.state.isCommand
      })); // executes the query

      onRunQuery();
    }; // Get value-related parameters


    _this.getParameterComponents = function () {
      if (_this.state.selectedValue.label === 'aggregatedStates') {
        return react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", null, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("span", {
          className: "gf-form-pre"
        }, "Value parameters"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", {
          className: "gf-form"
        }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(components_values_AggregatedStates__WEBPACK_IMPORTED_MODULE_4__["AggregatedStates"], {
          value: _this.state.parameterString,
          onChange: _this.onParametersChange
        })));
      } else {
        _this.setState({
          parameterString: ''
        });

        return react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", null);
      }
    }; // Update chart values. The value parameter is required to avoid
    // waiting for the state to be updated asynchronously


    _this.updateChart = function (value, parameterString) {
      if (parameterString === void 0) {
        parameterString = '';
      }

      var _a = _this.props,
          onChange = _a.onChange,
          query = _a.query,
          onRunQuery = _a.onRunQuery;
      onChange(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])(Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__assign"])({}, query), {
        location: _this.state.selectedLocation,
        asset: _this.state.selectedAsset,
        value: value,
        parameterString: parameterString,
        labelsField: _this.state.labelsField
      })); // executes the query

      onRunQuery();
    };

    _this.onLabelsFieldNameChange = function (event) {
      // Update the labels field
      _this.setState({
        labelsField: event.target.value
      });
    };

    _this.useColumnAsFieldNames = function (event) {
      // Update the chart
      _this.updateChart(_this.state.selectedValue, _this.state.parameterString);
    }; // Default values while we wait for the server's response
    // Using this.props.query... to restore the previous values


    var query = Object(lodash__WEBPACK_IMPORTED_MODULE_5__["defaults"])(_this.props.query, _types__WEBPACK_IMPORTED_MODULE_3__["defaultQuery"]);
    _this.state = {
      selectedLocation: query.location,
      selectedAsset: query.asset,
      selectedValue: query.value,
      labelsField: query.labelsField,
      locationOptions: [{
        label: '',
        value: 0
      }],
      assetOptions: [{
        label: '',
        value: 0
      }],
      valueOptions: [{
        label: '',
        value: 0
      }],
      parameterString: query.parameterString,
      isCommand: query.isCommand
    };
    return _this; // Get locations related to the configured customer
    // This assumes that the server resturns an array of strings:
    // ['location1', 'location2', ...]

    /* this.props.datasource.getLocations((locationArray: any[]) => { */

    /*   // Get default or previous value */

    /*   const locationIndex = this.state.selectedLocation.index; */

    /*   // Update state with new location options */

    /*   const newLocationLabel = locationArray[locationIndex]; */

    /*   const newLocationOptions = locationArray.map((location, index) => { */

    /*     return { label: location, value: index }; */

    /*   }); */

    /*   this.setState({ */

    /*     selectedLocation: { label: newLocationLabel, index: locationIndex }, */

    /*     locationOptions: newLocationOptions, */

    /*   }); */

    /*   // Get from server the assets related to the chosen location */

    /*   /1* this.props.datasource.getAssets(newLocationLabel, this.setAssetsOptions); *1/ */

    /* }); */
  }

  QueryEditor.prototype.render = function () {
    var customParameters = this.getParameterComponents();
    return react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", {
      className: "gf-form-group"
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("span", {
      className: "gf-form-pre"
    }, "Query parameters"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", {
      className: "gf-form"
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("label", {
      className: "gf-form-label"
    }, "Location"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Select"], {
      options: this.state.locationOptions,
      onChange: this.onLocationChange,
      value: this.state.selectedLocation.index
    }), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("label", {
      className: "gf-form-label"
    }, "Asset"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Select"], {
      options: this.state.assetOptions,
      onChange: this.onAssetChange,
      value: this.state.selectedAsset.index
    }), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("label", {
      className: "gf-form-label"
    }, "Value"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Select"], {
      options: this.state.valueOptions,
      onChange: this.onValueChange,
      value: this.state.selectedValue.index
    })), customParameters, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", {
      className: "gf-form"
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("label", {
      className: "gf-form-label with-5 query-keyword"
    }, "DEBUG COMMAND"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Switch"], {
      css: "",
      disabled: false,
      value: this.state.isCommand,
      onChange: this.onIsCommandChange
    })), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("span", {
      className: "gf-form-pre"
    }, "Transformations"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", {
      className: "gf-form"
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("label", {
      className: "gf-form-label"
    }, "Labels field"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Input"], {
      css: "",
      placeholder: "Name of the field to be used as column names.",
      onChange: this.onLabelsFieldNameChange,
      value: this.state.labelsField
    }), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Button"], {
      onClick: this.useColumnAsFieldNames
    }, "Apply")));
  };

  return QueryEditor;
}(react__WEBPACK_IMPORTED_MODULE_1__["PureComponent"]);



/***/ }),

/***/ "./components/values/AggregatedStates.tsx":
/*!************************************************!*\
  !*** ./components/values/AggregatedStates.tsx ***!
  \************************************************/
/*! exports provided: aggregatedStatesDefaultString, AggregatedStates */
/***/ (function(module, __webpack_exports__, __webpack_require__) {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "aggregatedStatesDefaultString", function() { return aggregatedStatesDefaultString; });
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "AggregatedStates", function() { return AggregatedStates; });
/* harmony import */ var tslib__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(/*! tslib */ "../node_modules/tslib/tslib.es6.js");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(/*! react */ "react");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _grafana_ui__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(/*! @grafana/ui */ "@grafana/ui");
/* harmony import */ var _grafana_ui__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__);


 // Sets the defaults when the component is loaded

function aggregatedStatesDefaultString() {
  return '&includeRunning=false';
}

var AggregatedStates =
/** @class */
function (_super) {
  Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__extends"])(AggregatedStates, _super);

  function AggregatedStates(props) {
    var _this = // Initialise
    _super.call(this, props) || this;

    _this.onIncludeRunningChange = function () {
      var parameterString = "&includeRunning=" + !_this.state.includeRunning;

      _this.setState({
        includeRunning: !_this.state.includeRunning
      });

      _this.props.onChange(parameterString);
    }; // Parse passed string and initialise state


    var initialState = false;

    try {
      var urlParameters = _this.props.value.split('&');

      urlParameters.map(function (parameter) {
        var keyValuePair = parameter.split('='); // Look for inlude running

        if (keyValuePair[0] === 'includeRunning') {
          if (keyValuePair[1] === 'true' || keyValuePair[1] === 'True') {
            initialState = true;
          }
        }
      });
    } catch (_a) {
      console.log('Wrong aggregatedStates parameters. It must either be includeRunning=true or includeRunning=false ');
    }

    _this.state = {
      includeRunning: initialState
    };
    return _this;
  }

  AggregatedStates.prototype.render = function () {
    return react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("div", {
      className: "gf-form"
    }, react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement("label", {
      className: "gf-form-label with-5 query-keyword"
    }, "Include Running"), react__WEBPACK_IMPORTED_MODULE_1___default.a.createElement(_grafana_ui__WEBPACK_IMPORTED_MODULE_2__["Switch"], {
      css: "",
      disabled: false,
      value: this.state.includeRunning,
      onChange: this.onIncludeRunningChange
    }));
  };

  return AggregatedStates;
}(react__WEBPACK_IMPORTED_MODULE_1__["Component"]);



/***/ }),

/***/ "./datasource.ts":
/*!***********************!*\
  !*** ./datasource.ts ***!
  \***********************/
/*! exports provided: DataSource */
/***/ (function(module, __webpack_exports__, __webpack_require__) {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "DataSource", function() { return DataSource; });
/* harmony import */ var tslib__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(/*! tslib */ "../node_modules/tslib/tslib.es6.js");
/* harmony import */ var _grafana_runtime__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(/*! @grafana/runtime */ "@grafana/runtime");
/* harmony import */ var _grafana_runtime__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_grafana_runtime__WEBPACK_IMPORTED_MODULE_1__);



var DataSource =
/** @class */
function (_super) {
  Object(tslib__WEBPACK_IMPORTED_MODULE_0__["__extends"])(DataSource, _super);

  function DataSource(instanceSettings) {
    return _super.call(this, instanceSettings) || this;
  }

  return DataSource;
}(_grafana_runtime__WEBPACK_IMPORTED_MODULE_1__["DataSourceWithBackend"]);



/***/ }),

/***/ "./module.ts":
/*!*******************!*\
  !*** ./module.ts ***!
  \*******************/
/*! exports provided: plugin */
/***/ (function(module, __webpack_exports__, __webpack_require__) {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "plugin", function() { return plugin; });
/* harmony import */ var _grafana_data__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(/*! @grafana/data */ "@grafana/data");
/* harmony import */ var _grafana_data__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_grafana_data__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _datasource__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(/*! ./datasource */ "./datasource.ts");
/* harmony import */ var _ConfigEditor__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(/*! ./ConfigEditor */ "./ConfigEditor.tsx");
/* harmony import */ var _QueryEditor__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(/*! ./QueryEditor */ "./QueryEditor.tsx");




var plugin = new _grafana_data__WEBPACK_IMPORTED_MODULE_0__["DataSourcePlugin"](_datasource__WEBPACK_IMPORTED_MODULE_1__["DataSource"]).setConfigEditor(_ConfigEditor__WEBPACK_IMPORTED_MODULE_2__["ConfigEditor"]).setQueryEditor(_QueryEditor__WEBPACK_IMPORTED_MODULE_3__["QueryEditor"]);

/***/ }),

/***/ "./types.ts":
/*!******************!*\
  !*** ./types.ts ***!
  \******************/
/*! exports provided: defaultQuery */
/***/ (function(module, __webpack_exports__, __webpack_require__) {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export (binding) */ __webpack_require__.d(__webpack_exports__, "defaultQuery", function() { return defaultQuery; });
var defaultQuery = {
  location: {
    label: '',
    index: 0
  },
  asset: {
    label: '',
    index: 0
  },
  value: {
    label: '',
    index: 0
  },
  parameterString: '',
  labelsField: '',
  isCommand: false
};

/***/ }),

/***/ "@grafana/data":
/*!********************************!*\
  !*** external "@grafana/data" ***!
  \********************************/
/*! no static exports found */
/***/ (function(module, exports) {

module.exports = __WEBPACK_EXTERNAL_MODULE__grafana_data__;

/***/ }),

/***/ "@grafana/runtime":
/*!***********************************!*\
  !*** external "@grafana/runtime" ***!
  \***********************************/
/*! no static exports found */
/***/ (function(module, exports) {

module.exports = __WEBPACK_EXTERNAL_MODULE__grafana_runtime__;

/***/ }),

/***/ "@grafana/ui":
/*!******************************!*\
  !*** external "@grafana/ui" ***!
  \******************************/
/*! no static exports found */
/***/ (function(module, exports) {

module.exports = __WEBPACK_EXTERNAL_MODULE__grafana_ui__;

/***/ }),

/***/ "lodash":
/*!*************************!*\
  !*** external "lodash" ***!
  \*************************/
/*! no static exports found */
/***/ (function(module, exports) {

module.exports = __WEBPACK_EXTERNAL_MODULE_lodash__;

/***/ }),

/***/ "react":
/*!************************!*\
  !*** external "react" ***!
  \************************/
/*! no static exports found */
/***/ (function(module, exports) {

module.exports = __WEBPACK_EXTERNAL_MODULE_react__;

/***/ })

/******/ })});;
//# sourceMappingURL=module.js.map