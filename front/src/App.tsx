import { useEffect, useState } from "react";
import "./App.css";
import { ToastContainer, toast } from "react-toastify";
import "react-toastify/dist/ReactToastify.css";

type Piece = "Empty" | "Pawn" | "Rook" | "Knight" | "Bishop" | "Queen" | "King";
type Side = "None" | "Black" | "White";
let initialBoard: Array<Array<{ piece: Piece; side: Side }>> = [];

for (let i = 0; i < 8; i++) {
  let row = [];
  for (let j = 0; j < 8; j++) {
    row.push({ piece: "Empty" as const, side: "None" as const });
  }
  initialBoard.push(row);
}
for (let i = 0; i < 8; i++) {
  // set color of powerful pieces
  initialBoard[0][i]["side"] = "Black";
  initialBoard[7][i]["side"] = "White";

  // initialize the pawns
  initialBoard[1][i]["side"] = "Black";
  initialBoard[1][i]["piece"] = "Pawn";
  initialBoard[6][i]["side"] = "White";
  initialBoard[6][i]["piece"] = "Pawn";
}
for (let i of [0, 7]) {
  initialBoard[i][0]["piece"] = "Rook";
  initialBoard[i][1]["piece"] = "Knight";
  initialBoard[i][2]["piece"] = "Bishop";
  initialBoard[i][3]["piece"] = "Queen";
  initialBoard[i][4]["piece"] = "King";
  initialBoard[i][5]["piece"] = "Bishop";
  initialBoard[i][6]["piece"] = "Knight";
  initialBoard[i][7]["piece"] = "Rook";
}

const pieceAssets = {
  pawn: <div>♟</div>,
  rook: <div>♜</div>,
  knight: <div>♞</div>,
  bishop: <div>♝ </div>,
  queen: <div>♛</div>,
  king: <div>♚</div>,
  empty: <div> </div>,
};

function App() {
  const notify = (message: string) => toast(message);
  const [board, setBoard] = useState(initialBoard);
  const [socket, setSocket] = useState<WebSocket | null>(null);
  const [socketActive, setSocketActive] = useState(false);
  const [sequenceNumber, setSequenceNumber] = useState(0);
  const [activePiece, setActivePiece] = useState<[number, number]>([-1, -1]);

  useEffect(() => {
    if (socket == null) {
      const sock = new WebSocket("ws://localhost:8080");
      setSocket(sock);

      sock.addEventListener("open", (event) => {
        console.log("Open Event Received: ", event);
        sock.send("TOKEN");
      });
      sock.addEventListener("message", (event) => {
        console.log("[From Server]", event.data);
        notify("Server says " + event.data.slice(0, 25));
        const splitted = event.data.split(" ");
        if (splitted[0] == "state") {
          setSequenceNumber(Number(splitted[1]));
          setBoard ((_b)=>{
            const receivedState = event.data.split(") (");
            const newBoard = [];
            for (let i = 0; i < 8; i++) {
              let row = [];
              for (let j = 0; j < 8; j++) {
                const [_hasMoved, side, piece] = receivedState[8 * i + j]
                  .replace(/state \d+ \(/, "")
                  .replace(")", "")
                  .split(", ");
                row.push({ piece,side });
              }
              newBoard.push(row);
            }
            return newBoard
          })
        } else if (splitted[0] == "othermove") {
          move(
            [Number(splitted[1]), Number(splitted[2])],
            [Number(splitted[3]), Number(splitted[4])],
          );
          setSequenceNumber((seqNum) => {
            return seqNum + 1;
          });
        } else if (splitted[0] == "notYourTurn") {
          notify("It is not your turn!")
        } else if (splitted[0] == "moveack") {
          setSequenceNumber((seqNum)=> seqNum + 1)
        }
      });
      return () => {
        sock.close();
      };
    }
  }, []);

  const move = (src: [number, number], dest: [number, number]) => {
    setBoard((board: typeof initialBoard) => {
      const finalBoard = JSON.parse(JSON.stringify(board))
      const src_piece = finalBoard[src[0]][src[1]];
      console.log("SRC", src_piece, src, dest)
      finalBoard[dest[0]][dest[1]] = {
        side: src_piece.side,
        piece: src_piece.piece,
      };
      finalBoard[src[0]][src[1]] = { side: "None", piece: "Empty" };
      console.log("DEBUG MOVE")
      console.log(board)
      console.log(finalBoard)
      return finalBoard;
    });
  };

  return (
    <>
      <ToastContainer />
      <div style={{ display: "flex", flexDirection: "column" }}>
        {board.map((row, i) => {
          return (
            <div
              style={{
                display: "flex",
                flexDirection: "row",
              }}
            >
              {row.map((p, j) => {
                let background = (i + j) % 2 == 1 ? "#555" : "#aaa";

                if (i == activePiece[0] && j == activePiece[1]) {
                  background = "#55aa55";
                }
                return (
                  <div
                    style={{
                      background,
                      color: p.side.toLowerCase(),
                      height: "5rem",
                      width: "5rem",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      fontSize: "3rem",
                    }}
                    onClick={() => {
                      // if activePiece is not set
                      if (activePiece[0] == -1 && activePiece[1] == -1) {
                        setActivePiece([i, j]);
                        return;
                      }
                      // if this is the active piece (trying to move somewhere else)
                      if (i == activePiece[0] && j == activePiece[1]) {
                        setActivePiece([-1, -1]);
                        return;
                      }

                      // move piece from activePiece -> i,j
                      // inform Server
                      move(activePiece, [i, j]);
                      socket!.send(
                        `${sequenceNumber + 1} ${activePiece[0]} ${activePiece[1]} ${i} ${j}`,
                      );
                      setActivePiece([-1, -1]);
                    }}
                  >
                    {
                      //@ts-ignore
                      pieceAssets[p.piece.toLowerCase()]
                    }
                  </div>
                );
              })}
            </div>
          );
        })}
      </div>
    </>
  );
}

export default App;
