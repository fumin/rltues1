Tic-Tac-Toe game server
-----

This is the source code for the game server hosting the Tic-Tac-Toe competition.
Clients participating in the competition should use the [Server Sent Events](http://www.html5rocks.com/en/tutorials/eventsource/basics/) protocol to receive game state updates and winner outcomes.
An example in Javascript demonstrating a typical game play can be found in /static/test.html.

## API

### /TTT
/TTT is the server sent events endpoint. Possible arguments are:

* room: the name of the game room
* player: circle or cross

There are three types of events:

* token: the authentication token used for subsequent submissions of game actions
* b: board state. A Tic-Tac-Toe board is represented as an array of length 9.
  Possible array element values are: "0" not occupied, "1" circle, and "2" cross.
  The board positions are indexed row by row, i.e. 0 is the top left corner, 1 is the top position, 4 is the center, and 8 is the bottom right corner.
* w: the winner of the game

Upon connection open, both the token and a board state event will be sent.
During every turn, a board state will be sent.
Upon game end, the winner will be announced through the w event.

Example:
`http://127.0.0.1:8080/TTT?player=cross&room=1444036341615`

### /TTTMove
/TTTMove submits an action to the server. Supported arguments are:

* room: the game room
* token: the authentication token returned by the /TTT endpoint
* player: circle or cross
* position: an integer representing the position on the board

Example:
`http://127.0.0.1:8080/TTTMove?token=B5xHdcw_Ww2u-kopnynAYw&player=cross&room=1444036341615&position=7`
