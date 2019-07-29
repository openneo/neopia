# neopia
This is part of the infrastructure for [Dress to Impress](https://impress.openneo.net)!

The _rest_ of the app is written in Ruby on Rails, which isn't great at blocking on async requests to other servers.
So, when users type their pet's name into the box on the homepage, we redirect them to this Go server instead,
which makes the request to Neopets.com, and redirects them back to the main app once it loads.

We'd previously imagined this being a cute general Neopets API (NeoAPI -> Neopia, get it?),
but that hasn't really panned out—this is really just a cute piece of infra to help Dress to Impress go quickly!
