# Metallō topic model explorer. 

GoLang App for working with high-dimensional data as produced by ToPān. It starts a server, allows one to serve the LDAVIS generated in ToPān, can read ToPān's theta file (which stores a probability distribution that can be seen as sparse high-dimensional document-vector embeddings), measures distances between documents (manhattan / JSD), and helps one to explore the high-dimensional space.

## How to use it

Download the zip, modify `config.json` with a text editor of your choice, and use one of the pre-build binaries (e.g. `metalloWin.exe`) or do your own build with `go build`. For the latter you need to have GoLang installed.

## End points

Once you have started it there are several endpoints. Some are relative easy to use:

- `/`: shows you that everything works.
- `/topic/{topic}/{count}`: allows you to access the passages based on their topic values; e.g.`/topic/13/20` will print the passages with the 20 highest values for topic 13. 
- `/view/{urn}/{count}`: shows you the most similar passages to a given passage in a GUI.
- `/view/{urn}/{count}/json`: sends the data above as a JSON response.
- `/theta/`: static directory that allows you to serve your theta files.
- `/ldavis/`: static directory that allows you to serve ToPān's LDA visualisation.

Others are for more experienced users:

- `/divergenceJS`: compares all passages with each other and sends a json file containing the result as response. This will run just on one core and depending on the size of your theta file will take a long time and produce an enormous json file. 
- `/divergenceCSV`: compares all passages with each other and produces several CSV files that contain their divergence. This is the preferred way of producing the divergence information for all passages. It will use all but one of the available cores of your system. It is much faster than `/divergenceJS`, but depending on your theta file, it might still take a long time. Progress is printed in the log-file.
- `/processed`: served directory containing the result of `/divergenceCSV`.
- `/static/`, `/js/`: served directories containing JavaScript and GoLang Templates.

## Config file

When you open `config.json`, you will see several parameters:

- `host`: wherever you want to host it. You probably don't need to change this, unless you want to host it on a server or change the `port`.
- `port`: wherever you want to host it locally. You probably don't need to change this, unless `3737` is already busy.
- `csv_source`: either web-address or filesystem location of the theta file.
- `local`: boolean value that indicates whether the theta file is stored locally or need to be retrieved from a web-address.
- `db`: boolean value that indicates whether you want to use Metallō's build-in BoltDB. If you want to run it on a server that might be useful, because than Metallō uses a lot less memory. It will also be significantly slower. If you set this to true, you will need to start Metallō with the `-loadDB` flag the first time you use a new theta file.
- `significance`: float value that marks a significant distance in one dimension.
- `dimWeight`: float value that helps interpretability. 
- `vizWeight`: float value that helps to increase the visual distance on the depicted graph.
- `distance`: selected distance measaure (`"jsd"` or `"manhattan"`).
- `divMax`: float value that determines which distances should be included when all passages are compared.
- `fileLimit`: integer that regulates the size of the CSV files that are produced when all passages are compared.

## Deprecated, but still gives you an impression

Older usage see [here](https://drive.google.com/file/d/1ybnjrTV6njYlQKddbCe2I6b2amhv1Rvx/view)
