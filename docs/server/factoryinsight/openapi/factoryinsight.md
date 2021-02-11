---
title: factoryinsight
language_tabs: []
toc_footers: []
includes: []
search: false
highlight_theme: darkula
headingLevel: 2

---

<section>
<h1 id="factoryinsight">factoryinsight v1.0</h1>

> TEST Scroll down for example requests and responses.

Base URLs:

* <a href="https://api.industrial-analytics.net/api/v1">https://api.industrial-analytics.net/api/v1</a>

</section>

<section>

# Authentication

- HTTP Authentication, scheme: basic

</section>

<section>
<h1 id="factoryinsight-default">Default</h1>

<section>

## GET /{customer}

<section>
<h3 id="get__{customer}-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|

</section>

<section>
<h3 id="get__{customer}-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|available locations for that customer|Inline|

<h3 id="get__{customer}-responseschema">Response Schema</h3>

<h3 id="get__{customer}-exampleresponses">Example responses</h3>

> 200 Response

```json
[
  "AachenPlant",
  "DuesseldorfPlant"
]
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}

<section>
<h3 id="get__{customer}_{location}-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|

</section>

<section>
<h3 id="get__{customer}_{location}-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|available assets for that customer and location|Inline|

<h3 id="get__{customer}_{location}-responseschema">Response Schema</h3>

<h3 id="get__{customer}_{location}-exampleresponses">Example responses</h3>

> 200 Response

```json
[
  "Warping",
  "Weaving"
]
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}

<section>
<h3 id="get__{customer}_{location}_{asset}-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|available datapoints for that customer, location and asset|Inline|

<h3 id="get__{customer}_{location}_{asset}-responseschema">Response Schema</h3>

<h3 id="get__{customer}_{location}_{asset}-exampleresponses">Example responses</h3>

> 200 Response

```json
[
  "state",
  "count",
  "currentState",
  "recommendation",
  "aggregatedStates",
  "timeRange",
  "availability",
  "performance",
  "oee",
  "productionSpeed",
  "shifts",
  "stateHistogram",
  "factoryLocations",
  "averageCleaningTime",
  "averageChangeoverTime",
  "upcomingMaintenanceActivities",
  "maintenanceComponents",
  "maintenanceActivities",
  "uniqueProducts",
  "orderTable",
  "orderTimeline",
  "process_SignalLamp",
  "process_TotalActivePower",
  "process_TotalApparentPower"
]
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/state

<section>
<h3 id="get__{customer}_{location}_{asset}_state-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|keepStatesInteger|query|boolean|false|If set to true all returned states will be numbers at not strings|*true*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_state-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns all states in the selected time range in a data format that can be consumed easily by Grafana|Inline|

<h3 id="get__{customer}_{location}_{asset}_state-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_state-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-AachenPlant-WeavingMachine-state",
    "timestamp"
  ],
  "datapoints": [
    [
      "No shift",
      1605187036148
    ],
    [
      "Producing",
      1605201132835
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/count

<section>
<h3 id="get__{customer}_{location}_{asset}_count-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_count-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the produced pieces in the selected time range in a data format that can be consumed easily by Grafana|Inline|

<h3 id="get__{customer}_{location}_{asset}_count-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_count-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-AachenPlant-WeavingMachine-count",
    "DCCAachen-AachenPlant-WeavingMachine-scrap",
    "timestamp"
  ],
  "datapoints": [
    [
      1250,
      0,
      1605201148990
    ],
    [
      625,
      0,
      1605201408149
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/currentState

<section>
<h3 id="get__{customer}_{location}_{asset}_currentstate-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_currentstate-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the current asset state in a data format that can be consumed easily by Grafana|Inline|

<h3 id="get__{customer}_{location}_{asset}_currentstate-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_currentstate-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-AachenPlant-WeavingMachine-state",
    "timestamp"
  ],
  "datapoints": [
    [
      "Unbekannter Zustand 1",
      1608298176825
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/recommendation

<section>
<h3 id="get__{customer}_{location}_{asset}_recommendation-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_recommendation-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the current recommendations for the asset in a data format that can be consumed easily by Grafana|Inline|

<h3 id="get__{customer}_{location}_{asset}_recommendation-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_recommendation-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "timestamp",
    "recommendationType",
    "recommendationValues",
    "recommendationTextEN",
    "recommendationTextDE",
    "diagnoseTextEN",
    "diagnoseTextDE"
  ],
  "datapoints": [
    [
      1600707538210,
      1,
      {
        "Threshold": 30,
        "StoppedForTime": 612685
      },
      "Start machine DCCAachen-Demonstrator or specify stop reason.",
      "Maschine DCCAachen-Demonstrator einschalten oder Stoppgrund auswählen.",
      "Machine DCCAachen-Demonstrator is not running since 612685 seconds (status: 8, threshold: 30)",
      "Maschine DCCAachen-Demonstrator steht seit 612685 Sekunden still (Status: 8, Schwellwert: 30)"
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/timeRange

<section>
<h3 id="get__{customer}_{location}_{asset}_timerange-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_timerange-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the first and last timepoint in the database in a data format that can be consumed easily by Grafana. This can be used to determine from and to parameters when you want to show all data|Inline|

<h3 id="get__{customer}_{location}_{asset}_timerange-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_timerange-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "firstTimestamp",
    "lastTimestamp"
  ],
  "datapoints": [
    [
      "2020-11-03T12:27:22Z",
      "2021-02-01T18:51:50Z"
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/aggregatedStates

<section>
<h3 id="get__{customer}_{location}_{asset}_aggregatedstates-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|includeRunning|query|boolean|true|If set to true the returned data will include running states as well. This makes sense for a pie chart showing the machine states, but for a pareto bar chart you would set this to false.|*true*|
|keepStatesInteger|query|boolean|false|If set to true all returned states will be numbers at not strings|*true*|
|aggregationType|query|integer|true|With this parameter you can specify how the data should be aggregated. 0 means aggregating it over the entire time range. 1 means aggregating it by hours in a day.|*1*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_aggregatedstates-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This sums up the duration of all states in the selected time range in a data format that can be consumed easily by Grafana. With aggregationType additional aggregations can be selected, e.g. grouping it additionally by the hour of the day. If aggregationType = 0, then the category column is omitted. There is still a bug, that keepStatesInteger is not properly working in some cases|Inline|

<h3 id="get__{customer}_{location}_{asset}_aggregatedstates-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data. duration is the amount of seconds.|

<h3 id="get__{customer}_{location}_{asset}_aggregatedstates-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    [
      "category",
      "state",
      "duration"
    ]
  ],
  "datapoints": [
    [
      9,
      150000,
      3638.4260000000004
    ],
    [
      9,
      160000,
      2001.471
    ],
    [
      10,
      150000,
      6185.870000000001
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/availability

<section>
<h3 id="get__{customer}_{location}_{asset}_availability-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_availability-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the availability in the selected time range in a data format that can be consumed easily by Grafana. The way how availability is defined can be configured in the database.|Inline|

<h3 id="get__{customer}_{location}_{asset}_availability-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_availability-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-Aachen-warping-availability"
  ],
  "datapoints": [
    [
      0.039953472102622754
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/performance

<section>
<h3 id="get__{customer}_{location}_{asset}_performance-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_performance-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the performance in the selected time range in a data format that can be consumed easily by Grafana. The way how performance is defined can be configured in the database.|Inline|

<h3 id="get__{customer}_{location}_{asset}_performance-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_performance-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-Aachen-warping-performance"
  ],
  "datapoints": [
    [
      0.039953472102622754
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/quality

<section>
<h3 id="get__{customer}_{location}_{asset}_quality-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_quality-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the quality in the selected time range in a data format that can be consumed easily by Grafana. Quality is defined by goodProducts / totalProducts|Inline|

<h3 id="get__{customer}_{location}_{asset}_quality-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_quality-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-Aachen-warping-quality"
  ],
  "datapoints": [
    [
      0.8
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/oee

<section>
<h3 id="get__{customer}_{location}_{asset}_oee-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_oee-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the OEE in the selected time range in a data format that can be consumed easily by Grafana. The result is then split up for each day. The way how OEE is defined can be configured in the database. There is an open issue here|Inline|

<h3 id="get__{customer}_{location}_{asset}_oee-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_oee-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-Aachen-warping-oee",
    "timestamp"
  ],
  "datapoints": [
    [
      "0.1779839097022094,",
      "2020-11-02T17:07:22Z"
    ],
    [
      "0.018431700156319477,",
      "2020-11-03T17:07:22Z"
    ],
    [
      "0.036663254805344575,",
      "2020-11-12T17:07:22Z"
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/shifts

<section>
<h3 id="get__{customer}_{location}_{asset}_shifts-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_shifts-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns all shifts in the selected time range in a data format that can be consumed easily by Grafana. The timestamp is returned as UNIX timestamp in milliseconds. shiftName = 0 means noShift, shiftName = 1 means shift.|Inline|

<h3 id="get__{customer}_{location}_{asset}_shifts-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_shifts-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "timestamp",
    "DCCAachen-Aachen-warping-shiftName"
  ],
  "datapoints": [
    [
      1608019200032,
      1
    ],
    [
      1608030000032,
      0
    ],
    [
      1608033600016,
      1
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/productionSpeed

<section>
<h3 id="get__{customer}_{location}_{asset}_productionspeed-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_productionspeed-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the production speed in the selected time range in a data format that can be consumed easily by Grafana. The production speed is in units/hour.|Inline|

<h3 id="get__{customer}_{location}_{asset}_productionspeed-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_productionspeed-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-Aachen-warping-speed",
    "timestamp"
  ],
  "datapoints": [
    [
      13560,
      1604077800000
    ],
    [
      1260,
      1604077860000
    ],
    [
      36360,
      1604077920000
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/qualityRate

<section>
<h3 id="get__{customer}_{location}_{asset}_qualityrate-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_qualityrate-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the quality rate in the selected time range in a data format that can be consumed easily by Grafana.|Inline|

<h3 id="get__{customer}_{location}_{asset}_qualityrate-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_qualityrate-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-Aachen-warping-qualityRate",
    "timestamp"
  ],
  "datapoints": [
    [
      0.8,
      1604077800000
    ],
    [
      0.85,
      1604077860000
    ],
    [
      0.82,
      1604077920000
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/stateHistogram

<section>
<h3 id="get__{customer}_{location}_{asset}_statehistogram-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|includeRunning|query|boolean|true|If set to true the returned data will include running states as well. This makes sense for a pie chart showing the machine states, but for a pareto bar chart you would set this to false.|*true*|
|keepStatesInteger|query|boolean|false|If set to true all returned states will be numbers at not strings|*true*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_statehistogram-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the frequency of each state in the selected time range in a data format that can be consumed easily by Grafana.|Inline|

<h3 id="get__{customer}_{location}_{asset}_statehistogram-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_statehistogram-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "state",
    "occurances"
  ],
  "datapoints": [
    [
      "Maschine läuft",
      147
    ],
    [
      "Unbekannter Stopp",
      6
    ],
    [
      "Mikrostopp",
      1
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/factoryLocations

<section>
<h3 id="get__{customer}_{location}_{asset}_factorylocations-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_factorylocations-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns all locations including a metric and geohash in the selected time range in a data format that can be consumed easily by Grafana. **Work in progress, currently only returning dummy data!**|Inline|

<h3 id="get__{customer}_{location}_{asset}_factorylocations-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_factorylocations-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "Location",
    "Metric",
    "Geohash"
  ],
  "datapoints": [
    [
      "Aachen",
      80,
      "u1h2fe"
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/averageCleaningTime

<section>
<h3 id="get__{customer}_{location}_{asset}_averagecleaningtime-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_averagecleaningtime-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the average cleaning time in the selected time range in a data format that can be consumed easily by Grafana. **Currently not working! See issue 93**|Inline|

<h3 id="get__{customer}_{location}_{asset}_averagecleaningtime-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_averagecleaningtime-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-Aachen-warping-averageCleaningTime",
    "timestamp"
  ],
  "datapoints": [
    [
      "1556,",
      "2020-11-02T17:07:22Z"
    ],
    [
      "1526,",
      "2020-11-03T17:07:22Z"
    ],
    [
      "756,",
      "2020-11-12T17:07:22Z"
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/averageChangeoverTime

<section>
<h3 id="get__{customer}_{location}_{asset}_averagechangeovertime-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_averagechangeovertime-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the average changeover time in the selected time range in a data format that can be consumed easily by Grafana. **Currently not working! See issue 93**|Inline|

<h3 id="get__{customer}_{location}_{asset}_averagechangeovertime-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_averagechangeovertime-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "DCCAachen-Aachen-warping-averageChangeoverTime",
    "timestamp"
  ],
  "datapoints": [
    [
      "1556,",
      "2020-11-02T17:07:22Z"
    ],
    [
      "1526,",
      "2020-11-03T17:07:22Z"
    ],
    [
      "756,",
      "2020-11-12T17:07:22Z"
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/upcomingMaintenanceActivities

<section>
<h3 id="get__{customer}_{location}_{asset}_upcomingmaintenanceactivities-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_upcomingmaintenanceactivities-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the upcoming maintenance activities in the selected time range in a data format that can be consumed easily by Grafana. The duration is in days. Negative values mean the maintenance activity is overdue. Status = 0 means the component is "critical". Status = 1 means the component is "orange" (under a third of the total runtime is remaining). Status = 2 means the component is "green" (over a third of the total runtime is remaining). Activity is here as a string. **Work in progress**|Inline|

<h3 id="get__{customer}_{location}_{asset}_upcomingmaintenanceactivities-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_upcomingmaintenanceactivities-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "Machine",
    "Component",
    "Activity",
    "Duration",
    "Status"
  ],
  "datapoints": [
    [
      "bernd2",
      "Pumpe 21",
      "Austausch",
      -69.5,
      0
    ],
    [
      "bernd2",
      "Pumpe 20",
      "Austausch",
      -79.08,
      0
    ],
    [
      "bernd2",
      "Pumpe 20",
      "Inspektion",
      -61.06,
      0
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/maintenanceComponents

<section>
<h3 id="get__{customer}_{location}_{asset}_maintenancecomponents-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_maintenancecomponents-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns all existing maintenance components for that asset. **This is not parsable by Grafana, therefore maintenanceComponents is not shown in the drop down menu.** **Work in progress**|Inline|

<h3 id="get__{customer}_{location}_{asset}_maintenancecomponents-responseschema">Response Schema</h3>

Status Code **200**

*the column names*

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|

<h3 id="get__{customer}_{location}_{asset}_maintenancecomponents-exampleresponses">Example responses</h3>

> 200 Response

```json
[
  "Pumpe 20",
  "Pumpe 21"
]
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/maintenanceActivities

<section>
<h3 id="get__{customer}_{location}_{asset}_maintenanceactivities-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_maintenanceactivities-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This shows all past maintenance activities in a data format that can be consumed easily by Grafana. Activity is here an integer symbolizing a string. The timestamp is a UNIX timestamp in milliseconds **Work in progress**|Inline|

<h3 id="get__{customer}_{location}_{asset}_maintenanceactivities-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_maintenanceactivities-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "Component",
    "Activity",
    "Timestamp"
  ],
  "datapoints": [
    [
      "Pumpe 20",
      0,
      1605178879502
    ],
    [
      "Pumpe 21",
      1,
      1605189835901
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/uniqueProducts

<section>
<h3 id="get__{customer}_{location}_{asset}_uniqueproducts-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_uniqueproducts-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the all unique products in the selected time range in a data format that can be consumed easily by Grafana. The data model behind unique products is explained at mqtt.md|Inline|

<h3 id="get__{customer}_{location}_{asset}_uniqueproducts-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_uniqueproducts-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "UID",
    "Timestamp begin",
    "Timestamp end",
    "Product ID",
    "Is Scrap",
    "Quality class",
    "Station ID"
  ],
  "datapoints": [
    [
      "161063193099727336211610631932133",
      1610631930997,
      1610631932133,
      "test123",
      false,
      "",
      "1a"
    ],
    [
      "16106319890449364101610631989682",
      1610631989044,
      1610631989682,
      "test123",
      false,
      "",
      "1a"
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/orderTable

<section>
<h3 id="get__{customer}_{location}_{asset}_ordertable-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_ordertable-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns all orders with additional information in the selected time range in a data format that can be consumed easily by Grafana. The backend calculates automatically for each order how many pieces were actually produced, what the difference between target and actual duration is and how long the machine has actually been running, how long the total changeover time was, etc. All durations are in seconds.|Inline|

<h3 id="get__{customer}_{location}_{asset}_ordertable-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_ordertable-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "Order ID",
    "Product ID",
    "Begin",
    "End",
    "Target units",
    "Actual units",
    "Target duration in seconds",
    "Actual duration in seconds",
    "Target time per unit in seconds",
    "Actual time per unit in seconds",
    "Producing",
    "Producing at lower than set speed",
    "No data",
    "Unknown stop",
    "Microstop",
    "Inlet jam",
    "Outlet jam",
    "Congestion in the bypass flow",
    "Other material issues",
    "Changeover",
    "Cleaning",
    "Emptying",
    "Setting up",
    "Operator missing",
    "Break",
    "No shift",
    "No order",
    "Equipment failure",
    "External failure",
    "External interference",
    "Maintenance",
    "Other technical issue",
    "Asset"
  ],
  "datapoints": [
    [
      "10463444",
      "product10463444",
      1595824438000,
      1595825648000,
      1,
      0,
      0,
      1210,
      827,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      "redacted-redacted"
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/orderTimeline

<section>
<h3 id="get__{customer}_{location}_{asset}_ordertimeline-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_ordertimeline-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns all orders in the selected time range for the discrete panel in Grafana.|Inline|

<h3 id="get__{customer}_{location}_{asset}_ordertimeline-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_ordertimeline-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "timestamp",
    "redacted-redacted-redacted-order"
  ],
  "datapoints": [
    [
      1592500042000,
      "noOrder"
    ],
    [
      1595824438000,
      "104638"
    ],
    [
      1595825648000,
      "noOrder"
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

<section>

## GET /{customer}/{location}/{asset}/process_{processValue}

<section>
<h3 id="get__{customer}_{location}_{asset}_process_{processvalue}-parameters">Parameters</h3>

|Name|In|Type|Required|Description|Example|
|---|---|---|---|---|---|
|customer|path|string|true|name of the customer|*DCCAachen*|
|location|path|string|true|name of the location|*AachenPlant*|
|asset|path|string|true|name of the asset|*WeavingMachine*|
|from|query|string(date)|true|Get data from this timestamp on (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|to|query|string(date)|true|Get data till this timestamp (in RFC3339 format)|*2020-11-10T00:00:00.000Z*|
|processValue|query|string|true|name of the process value to retrieve data|*energyConsumption*|

</section>

<section>
<h3 id="get__{customer}_{location}_{asset}_process_{processvalue}-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|This returns the selected process value in the selected time range in a data format that can be consumed easily by Grafana.|Inline|

<h3 id="get__{customer}_{location}_{asset}_process_{processvalue}-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» columnNames|[string]|false|none|the column names|
|» datapoints|[array]|false|none|the returned data|

<h3 id="get__{customer}_{location}_{asset}_process_{processvalue}-exampleresponses">Example responses</h3>

> 200 Response

```json
{
  "columnNames": [
    "timestamp",
    "DCCAachen-Aachen-warping-Fadenueber_link_aktiv"
  ],
  "datapoints": [
    [
      1606236115808,
      1
    ],
    [
      1606236192305,
      0
    ],
    [
      1606236217103,
      0
    ]
  ]
}
```

</section>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
BasicAuth
</aside>

</section>

</section>

