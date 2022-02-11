---
title: "Open source in Industrial IoT: an open and robust infrastructure instead of reinventing the wheel."
linkTitle: "Open Source in Industrial IoT"
author: Jeremy Theocharis
weight: 5
description: >
  How we are keeping up with the established players in Industrial IoT and why we believe the United Manufacturing Hub is changing the future of Industrial IoT and Industry 4.0 with the help of Open Source.
---

{{< imgproc christopher Fit "600x600" >}}Image author: Christopher Burns from Unsplash{{< /imgproc >}}

How do we keep up with the big players in the industry despite limited resources and small market share? The best way to do this is to break new ground and draw on the collective experience of organizations and their specialists instead of trying to reinvent the wheel.

The collaborative nature of open source enables companies and individuals alike to turn their visions into reality and keep up with established players such as Siemens, Microsoft, and Rockwell, even without a large number of programmers and engineers. This is the path we are taking at United Manufacturing Hub.

Open source software has long since outgrown the insider stage and has become a veritable trend that is becoming the standard in more and more industries. Many, in the IT world, common and intensively used applications (e.g. [Kubernetes](https://github.com/kubernetes/kubernetes), [TensorFlow](https://github.com/tensorflow/tensorflow), [f-prime by NASA](https://github.com/nasa/fprime) [^technologies]) have nowadays emerged in a collaborative approach and are available for free.

{{< imgproc PIA23962 Fit "400x400" >}}Open-Source on Mars: the Mars helicopter Ingenuity, relies heaviliy on open-source components like f-prime. Image author: JPL/NASA{{< /imgproc >}} 

Typically, these applications are not yet ready for production or Industry 4.0 use. Some, such as [Grafana](https://github.com/grafana/grafana), are intended for completely different industries (Observability & Monitoring). 

However, the source code of these software projects is freely accessible to everyone and can be individually adapted to specific needs. Thus, the application in the Industrial IoT is also no problem. In part, those applications were programmed over decades [^postgresql] by several thousand developers and are continuously developed further [^kubernetes].

[^postgresql]: https://www.postgresql.org/docs/current/history.html

[^kubernetes]: https://github.com/kubernetes/kubernetes

## The status quo

Today, it is common to develop proprietary Industrial IoT programs and software platforms - the opposite of open source. 

A reason behind is, that companies do not want to have foreign code written into their applications and they want to offer the customer a self-made, end-to-end solution. 

It is common for a team of over 20 or even 30 people to be assigned to develop a dashboard or IoT gateway, with the focus on a pretty looking (usually self-branded) user interface (UI) and design. Existing open source solutions or automation standards are rarely built upon.

Self-developed, in-house architectures are often strongly influenced by company-specific know-how and therefore usually also favor the company's own products and services in their interfaces. 

**The result:** the wheel is often reinvented in both the software and hardware areas. The resulting architectures create a lock-in effect that leads to a dependency of the manufacturing companies on their software and hardware suppliers.

### Reinventing the wheel: The software world

In our opinion, good examples in the category "reinvented the wheel" from the software world are:

1. Self-developed visualizations such as [visualizations from InfluxDB](https://www.influxdata.com/how-to-visualize-time-series-data/), [PI Vision from OSIsoft](https://www.osisoft.com/pi-system/pi-core/visualization) or [WAGO IoT Cloud Visualization](https://www.wago.com/de/offene-automatisierung/cloud-automatisierung/automatisierung-wago-cloud) (instead of *Grafana*).

2. Flow-based low code / no code apps such as [Wire Graph by Eurotech](https://esf.eurotech.com/v5.0.0/docs/kura-wires-overview) (instead of *node-red*)

3. The bulk of Industrial IoT platforms that are claiming to be a "one-stop solution." Such platforms are trying to cover every aspect from data acquisition, over processing, to visualization with in-house solutions (instead of relying on established technologies and just filling the gaps in the stack).

Both *Grafana* and *node-red* are highly professional solutions in their respective fields, which have already been used in various software projects for several years. Orchestrating such specialized applications means that offered and tested solutions can be put to good use.

### Reinventing the wheel: The hardware world 

There are numerous examples in the Industrial IoT hardware world where there is a conscious or unconscious deviation from established industry standards of the automation industry.

We have particularly noticed this with vendors in the field of *Overall Equipment Effectiveness (OEE)* and production overviews. Although they usually have very good dashboards, they still rely on self-developed microcontrollers combined with consumer tablets (instead of established automation standards such as a PLC or an industrial edge PC) for the hardware. In this case, the microcontroller, usually called *IoT gateway*, is considered a black box, and the end customer only gets access to the device in rare cases.

**The advantages cannot be denied:** 
1. the system is easy to use, 
2. usually very inexpensive, 
3. and requires little prior knowledge. 
 
**Unfortunately, these same advantages can also become disadvantages:** 

1. the house system integrator and house supplier is not able to work with the system, as it has been greatly reduced for simplicity. 
2. all software extensions and appearing problems, such as integrating software like an ERP system with the rest of the IT landscape, must be discussed with the respective supplier. This creates a one-sided market power (see also Lock-In).

Another problem that arises when deviating from established automation standards: **a lack of reliability**.

Normally, the system always need to work because failures lead to production downtime (the operator must report the problem). The machine operator just wants to press a button to get a stop reason or the desired information. He does not want to deal with WLAN problems, browser update or updated privacy policies on the consumer tablet.

### The strongest argument: Lock-In

In a newly emerging market, it is especially important for a manufacturing company not to make itself dependent on individual providers. Not only to be independent if a product/company is discontinued but also to be able to change providers at any time.

Particularly *pure* SaaS (Software-as-a-Service) providers should be handled with caution:

- A SaaS offering typically uses a centralized cloud-based server infrastructure for multiple customers simultaneously. By its very nature, this makes it difficult to integrate into the IT landscape, e.g., to link with the MES system installed locally in the factory. 
- In addition, a change of provider is practically only possible with large-scale reconfiguration/redevelopment.
- Lastly, there is a concern regarding the data ownership and security of closed systems and multiple SaaS offerings. 

Basically, exaggerating slightly to make the point, it is important to avoid highly sensitive production data with protected process parameters getting to foreign competitors.

One might think that the manufacturing company is initially entitled to all rights to the data - after all, it is the company that "produced" the data. 

In fact, according to the current situation, there is no comprehensive legal protection of the data, at least in Germany, if this is not explicitly regulated by contract, as the *Verband der deutschen Maschinenbauer (VDMA)* (Association of German Mechanical Engineering Companies) admits [^VDMA1].

Even when it comes to data security, some people feel queasy about handing over their data to someone else, possibly even a US startup. Absolutely rightly so, says the *VDMA*, because companies based in the USA are obliged to allow US government authorities access to the data at any time [^VDMA2].

[^VDMA1]: *Leitfaden Datennutzung. Orientierungshilfe zur Vertragsgestaltung für den Mittelstand.* Published by VDMA in 2019.

[^VDMA2]: *Digitale Marktabschottung: Auswirkungen von Protektionismus auf Industrie 4.0 * Published by VDMA's Impulse Foundation in 2019.

**An open source project can give a very good and satisfactory answer here:** 

> *United Manufacturing Hub* users can always develop the product further without the original developers, as the source code is fully open and documented.
> 
> All subcomponents are fully open and run on almost any infrastructure, from the cloud to a Raspberry Pi, always giving the manufacturing company control over all its data. 
>
> Interfaces with other systems are either included directly, greatly simplifying their development, or can be retrofitted themselves without being nailed down to specific programming languages.

## Unused potential

In the age of Industry 4.0, the top priority is for companies to operate as efficiently as possible by taking full advantage of their potential. 

Open source software, unlike classic proprietary software, enables this potential to be fully exploited. Resources and hundreds of man-hours can be saved by using free solutions and standards from the automation industry.

Developing and offering a proprietary dashboard or IoT gateway that is reliable, stable, and free of bugs is wasting valuable time.

Another hundred, if not a thousand, man-hours are needed until all relevant features such as single sign-on, user management, or logging are implemented. Thus, it is not uncommon that even large companies, the market leaders in the industry, do not operate efficiently, and the resulting products are in the 6-to-7-digit price range.

But the efficiency goes even further: 

Open source solutions also benefit from the fact that a community is available to help with questions. This service is rarely available with proprietary solutions. All questions and problems must be discussed with the multi-level support hotline instead of simply Googling the solution. 

And so, unfortunately, most companies take a path that is anything but efficient. **But isn’t there a better way?**

## United Manufacturing Hub’s open source approach.

Who says that you have to follow thought patterns or processes that everyone else is modeling? Sometimes it’s a matter of leaving established paths, following your own convictions, and initiating a paradigm shift. That is the approach we are taking.

We cannot compete with the size and resources of the big players. That is why we do not even try to develop in one or two years, with a team of 20 to 30 programmers what large companies have developed in hundreds of thousands of hours.

But that's not necessary because the resulting product is unlikely to keep up with the open source projects or established automation standards. That is why the duplicated work is not worth the struggle .

The open source software code is freely accessible and thus allows maximum transparency and, at the same time, security. It offers a flexibility that is not reached by programs developed in the traditional way. By using open source software, the United Manufacturing Hub is taking an efficient way of developing. It allows us to offer a product of at least equal value but with considerably fewer development costs.

{{< imgproc dashboard Fit "600x600" >}}Example OEE dashboard created in Grafana{{< /imgproc >}}

## Simplicity and efficiency in the age of Industrial IoT.

At United Manufacturing Hub, we combine open source technologies with industry-specific requirements. To do this, we draw on established software such as Docker, Kubernetes or Helm [^technologies] and create, for example, data models, algorithms, and KPIs (e.g. [the UMH data model](https://docs.umh.app/docs/concepts/mqtt/), the [factoryinsight and mqtt-to-postresql](https://docs.umh.app/docs/concepts/) components) that are needed in the respective industries.

By extracting all data from machine controls (OPC/UA, etc.), we ensure the management and distribution of data on the store floor. Also, if additional data is needed, we offer individual solutions using industry-specific certified sensor retrofit kits, for example, at a [steel manufacturer](https://docs.umh.app/docs/examples/flame-cutting/). More on this in one of the later parts of this series.

## Summary

Why should we reinvent the wheel when we can focus our expertise on the areas we can provide the most value to our customers?

**Leveraging open source solutions allow us to expose a stable and robust infrastructure that enables our customers to meet the challenges of Industrial IoT.** 

Because, in fact, manufacturing and Industrial IoT is not about developing new software at the drop of a hat. It is more about solving individual problems and challenges. This is done by drawing on a global network of experts who have developed special applications in their respective fields. These applications allow all hardware and software components to be quickly and easily established in the overall architecture through a large number of interfaces.

[^technologies]: For the common technologies see also [Understanding the technologies](/docs/getting-started/understanding-the-technologies).
