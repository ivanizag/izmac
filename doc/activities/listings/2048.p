program Game2048;

const
  TileSize = 52;
  Gap = 6;
  BoardLeft = 8;
  BoardTop = 30;
  WindowWidth = 254;
  WindowHeight = 276;
  AppleID = 128;
  FileID = 129;
  EditID = 130;
  NewItem = 1;
  QuitItem = 3;
  LeftArrow = 28;
  RightArrow = 29;
  UpArrow = 30;
  DownArrow = 31;

type
  Board = array[1..4, 1..4] of LongInt;
  Four = array[1..4] of LongInt;

var
  tiles: Board;
  score: LongInt;
  won, over, quitting, editEnabled: Boolean;
  window: WindowPtr;
  appleMenu, fileMenu, editMenu: MenuHandle;

procedure AddTile;
  var
    row, col, empty, pick: Integer;
begin
  empty := 0;
  for row := 1 to 4 do
    for col := 1 to 4 do
      if tiles[row, col] = 0 then
        empty := empty + 1;
  if empty > 0 then
    begin
      pick := Abs(Random) mod empty;
      for row := 1 to 4 do
        for col := 1 to 4 do
          if tiles[row, col] = 0 then
            begin
              if pick = 0 then
                if Abs(Random) mod 10 = 0 then
                  tiles[row, col] := 4
                else
                  tiles[row, col] := 2;
              pick := pick - 1;
            end;
    end;
end;

procedure NewGame;
  var
    row, col: Integer;
begin
  for row := 1 to 4 do
    for col := 1 to 4 do
      tiles[row, col] := 0;
  score := 0;
  won := false;
  over := false;
  AddTile;
  AddTile;
end;

function SlideLine (var squares: Four): Boolean;
  var
    result: Four;
    i, count: Integer;
    joined: Boolean;
begin
  for i := 1 to 4 do
    result[i] := 0;
  count := 0;
  joined := false;
  for i := 1 to 4 do
    if squares[i] <> 0 then
      if (count > 0) and not joined and (result[count] = squares[i]) then
        begin
          result[count] := result[count] * 2;
          score := score + result[count];
          if result[count] = 2048 then
            won := true;
          joined := true;
        end
      else
        begin
          count := count + 1;
          result[count] := squares[i];
          joined := false;
        end;
  SlideLine := false;
  for i := 1 to 4 do
    begin
      if result[i] <> squares[i] then
        SlideLine := true;
      squares[i] := result[i];
    end;
end;

function Slide (key: Integer): Boolean;
  var
    k, i, row, col: Integer;
    squares: Four;
    moved: Boolean;

  procedure Square (k, i: Integer; var row, col: Integer);
  begin
    case key of
      LeftArrow:
        begin
          row := k;
          col := i;
        end;
      RightArrow:
        begin
          row := k;
          col := 5 - i;
        end;
      UpArrow:
        begin
          row := i;
          col := k;
        end;
      DownArrow:
        begin
          row := 5 - i;
          col := k;
        end;
    end;
  end;

begin
  moved := false;
  for k := 1 to 4 do
    begin
      for i := 1 to 4 do
        begin
          Square(k, i, row, col);
          squares[i] := tiles[row, col];
        end;
      if SlideLine(squares) then
        moved := true;
      for i := 1 to 4 do
        begin
          Square(k, i, row, col);
          tiles[row, col] := squares[i];
        end;
    end;
  Slide := moved;
end;

function CanMove: Boolean;
  var
    row, col: Integer;
begin
  CanMove := false;
  for row := 1 to 4 do
    for col := 1 to 4 do
      begin
        if tiles[row, col] = 0 then
          CanMove := true;
        if (col < 4) and (tiles[row, col] = tiles[row, col + 1]) then
          CanMove := true;
        if (row < 4) and (tiles[row, col] = tiles[row + 1, col]) then
          CanMove := true;
      end;
end;

procedure DrawTile (row, col: Integer);
  var
    box, plate: Rect;
    number: Str255;
    value: LongInt;
    light: Pattern;
begin
  SetRect(box, 0, 0, TileSize, TileSize);
  OffsetRect(box, BoardLeft + Gap + (col - 1) * (TileSize + Gap), BoardTop + Gap + (row - 1) * (TileSize + Gap));
  EraseRoundRect(box, 12, 12);
  value := tiles[row, col];
  if value = 0 then
    begin
      PenPat(gray);
      FrameRoundRect(box, 12, 12);
      PenNormal;
    end
  else
    begin
      if value >= 512 then
        FillRoundRect(box, 12, 12, black)
      else if value >= 128 then
        FillRoundRect(box, 12, 12, dkGray)
      else if value >= 32 then
        FillRoundRect(box, 12, 12, gray)
      else if value >= 8 then
        FillRoundRect(box, 12, 12, ltGray)
      else if value = 4 then
        begin
          StuffHex(@light, '8800220088002200');
          FillRoundRect(box, 12, 12, light);
        end;
      FrameRoundRect(box, 12, 12);
      NumToString(value, number);
      if (value >= 4) and (value < 512) then
        begin
          SetRect(plate, 0, 0, StringWidth(number) + 10, 18);
          OffsetRect(plate, box.left + (TileSize - plate.right) div 2, box.top + (TileSize - 18) div 2);
          EraseRoundRect(plate, 8, 8);
          FrameRoundRect(plate, 8, 8);
        end;
      if value >= 512 then
        TextMode(srcBic);
      MoveTo(box.left + (TileSize - StringWidth(number)) div 2, box.top + TileSize div 2 + 5);
      DrawString(number);
      TextMode(srcOr);
    end;
end;

procedure DrawScore;
  var
    strip: Rect;
    number: Str255;
begin
  SetRect(strip, 0, 0, WindowWidth, BoardTop);
  EraseRect(strip);
  NumToString(score, number);
  MoveTo(BoardLeft + Gap, 20);
  DrawString(concat('Score: ', number));
  if over then
    number := 'Game over'
  else if won then
    number := 'You made 2048!'
  else
    number := '';
  MoveTo(WindowWidth - BoardLeft - Gap - StringWidth(number), 20);
  DrawString(number);
end;

procedure DrawBoard;
  var
    row, col: Integer;
begin
  SetPort(window);
  DrawScore;
  for row := 1 to 4 do
    for col := 1 to 4 do
      DrawTile(row, col);
end;

procedure ShowAbout;
  var
    box: Rect;
    about: WindowPtr;
begin
  HiliteMenu(0);
  SetRect(box, 106, 110, 406, 190);
  about := NewWindow(nil, box, '', true, dBoxProc, WindowPtr(-1), false, 0);
  SetPort(about);
  MoveTo(20, 30);
  DrawString('2048, written in THINK Pascal.');
  MoveTo(20, 55);
  DrawString('Click to go back to the game.');
  repeat
    SystemTask;
  until Button;
  repeat
  until not Button;
  FlushEvents(everyEvent, 0);
  DisposeWindow(about);
  SetPort(window);
end;

procedure DoMenu (choice: LongInt);
  var
    item: Integer;
    name: Str255;
begin
  item := LoWord(choice);
  case HiWord(choice) of
    AppleID:
      if item = 1 then
        ShowAbout
      else
        begin
          GetItem(appleMenu, item, name);
          item := OpenDeskAcc(name);
        end;
    FileID:
      if item = NewItem then
        begin
          NewGame;
          DrawBoard;
        end
      else if item = QuitItem then
        quitting := true;
    EditID:
      if SystemEdit(item - 1) then
        ;
  end;
  HiliteMenu(0);
end;

procedure DoKey (key: Integer);
begin
  case chr(key) of
    'a', 'A':
      key := LeftArrow;
    'd', 'D':
      key := RightArrow;
    'w', 'W':
      key := UpArrow;
    's', 'S':
      key := DownArrow;
    otherwise
  end;
  if not over and ((key = LeftArrow) or (key = RightArrow) or (key = UpArrow) or (key = DownArrow)) then
    if Slide(key) then
      begin
        AddTile;
        over := not CanMove;
        DrawBoard;
      end;
end;

procedure DoMouseDown (event: EventRecord);
  var
    part: Integer;
    which: WindowPtr;
    limits: Rect;
begin
  part := FindWindow(event.where, which);
  case part of
    inMenuBar:
      DoMenu(MenuSelect(event.where));
    inSysWindow:
      SystemClick(event, which);
    inDrag:
      begin
        limits := screenBits.bounds;
        InsetRect(limits, 4, 4);
        DragWindow(which, event.where, limits);
      end;
    inGoAway:
      if TrackGoAway(which, event.where) then
        quitting := true;
    inContent:
      if which <> FrontWindow then
        SelectWindow(which);
  end;
end;

procedure UpdateEditMenu;
  var
    front: WindowPeek;
    forAccessory: Boolean;
begin
  front := WindowPeek(FrontWindow);
  forAccessory := (front <> nil) and (front^.windowKind < 0);
  if forAccessory <> editEnabled then
    begin
      editEnabled := forAccessory;
      if editEnabled then
        EnableItem(editMenu, 0)
      else
        DisableItem(editMenu, 0);
      DrawMenuBar;
    end;
end;

procedure Setup;
  var
    box: Rect;
    title: Str255;
begin
  InitGraf(@thePort);
  InitFonts;
  InitWindows;
  InitMenus;
  TEInit;
  InitDialogs(nil);
  InitCursor;
  FlushEvents(everyEvent, 0);
  title := ' ';
  title[1] := chr(appleMark);
  appleMenu := NewMenu(AppleID, title);
  AppendMenu(appleMenu, 'About 2048...;(-');
  AddResMenu(appleMenu, 'DRVR');
  InsertMenu(appleMenu, 0);
  fileMenu := NewMenu(FileID, 'File');
  AppendMenu(fileMenu, 'New Game/N;(-;Quit/Q');
  InsertMenu(fileMenu, 0);
  editMenu := NewMenu(EditID, 'Edit');
  AppendMenu(editMenu, 'Undo/Z;(-;Cut/X;Copy/C;Paste/V;Clear');
  InsertMenu(editMenu, 0);
  DisableItem(editMenu, 0);
  editEnabled := false;
  DrawMenuBar;
  SetRect(box, 0, 0, WindowWidth, WindowHeight);
  OffsetRect(box, (512 - WindowWidth) div 2, 50);
  window := NewWindow(nil, box, '2048', true, noGrowDocProc, WindowPtr(-1), true, 0);
  SetPort(window);
  TextFont(systemFont);
  randSeed := TickCount;
  NewGame;
end;

procedure Run;
  var
    event: EventRecord;
begin
  quitting := false;
  repeat
    SystemTask;
    UpdateEditMenu;
    if GetNextEvent(everyEvent, event) then
      case event.what of
        mouseDown:
          DoMouseDown(event);
        keyDown, autoKey:
          if BitAnd(event.modifiers, cmdKey) <> 0 then
            DoMenu(MenuKey(chr(BitAnd(event.message, charCodeMask))))
          else
            DoKey(BitAnd(event.message, charCodeMask));
        updateEvt:
          begin
            BeginUpdate(WindowPtr(event.message));
            DrawBoard;
            EndUpdate(WindowPtr(event.message));
          end;
      end;
  until quitting;
end;

begin
  Setup;
  Run;
end.
