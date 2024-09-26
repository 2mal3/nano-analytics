# nano-analytics

An extremely lightweight and privacy-preserving analytics platform for self-hosting. It's easy to set up and integrate into your services to anonymously track device, browser, country, date, and your own custom user actions. The statistics are then easily accessible through a web UI.
The main idea of this application is based on [this](https://herman.bearblog.dev/how-bear-does-analytics-with-css/) very interesting article from Herman's blog.

The statistics can be viewed at the `http://<server address>/stats` url with the correct credentials.

## Setup The Server

I highly recommend using Docker (image: `ghcr.io/2mal3/nano-analytics:latest`).

### Required Configuration

- environment (env) variable: `ADMIN_USERNAME` (default `admin`)
- env variable: `ADMIN_PASSWORD_HASH` (bcrypt hash of the password, can be generated ith `mkpasswd -m bcrypt "<password>"`)
- forward port `1323`
- volume for `/app/database`

## Integration Into Your App

Somehow call the URL `http[s]://<server address>/track/<your application identifier>[?action=<a special action>&referrer=<the website referrer>]` from the client. Where the application identifier and the action can be any string of your choice.

For example, a website could do this with the following css class:

```css
body:hover {
    border-image: url("<url>");
}
```
