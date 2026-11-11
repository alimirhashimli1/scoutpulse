import { Injectable } from '@angular/core';
import { Observable, firstValueFrom } from 'rxjs';

import { GameRooms } from '../games';
import { GuessResult, RoomSeat, RoomSettings, RoomSnapshot } from '../../models/games';
import { HttpRepository } from './http-repository';

@Injectable()
export class HttpGameRooms extends HttpRepository implements GameRooms {
  protected readonly base = this.api.football;

  private room(code: string, ...rest: string[]): string {
    return this.url('games', 'played-for-both', 'rooms', ...(code ? [code] : []), ...rest);
  }

  create(name: string): Promise<RoomSeat> {
    return this.post<RoomSeat>(this.room(''), { name });
  }

  join(code: string, name: string): Promise<RoomSeat> {
    return this.post<RoomSeat>(this.room(code, 'players'), { name });
  }

  configure(seat: RoomSeat, settings: RoomSettings): Promise<void> {
    return this.noContent(
      this.http.put(
        this.room(seat.code, 'settings'),
        { token: seat.token, ...settings },
        {
          responseType: 'text',
        },
      ),
    );
  }

  start(seat: RoomSeat): Promise<void> {
    return this.act(seat, 'start');
  }

  next(seat: RoomSeat): Promise<void> {
    return this.act(seat, 'next');
  }

  stop(seat: RoomSeat): Promise<void> {
    return this.act(seat, 'stop');
  }

  guess(seat: RoomSeat, text: string): Promise<GuessResult> {
    return this.post<GuessResult>(this.room(seat.code, 'guesses'), { token: seat.token, text });
  }

  /**
   * A beacon when the browser has one: it is delivered even as the tab
   * closes, which a normal request is not.
   */
  leave(seat: RoomSeat): void {
    const url = this.room(seat.code, 'leave');
    const body = JSON.stringify({ token: seat.token });
    if (globalThis.navigator?.sendBeacon?.(url, body)) return;
    void fetch(url, { method: 'POST', body, keepalive: true }).catch(() => undefined);
  }

  poll(seat: RoomSeat): Promise<RoomSnapshot> {
    return this.getOne<RoomSnapshot>(this.room(seat.code), { token: seat.token });
  }

  eventsUrl(seat: RoomSeat): string {
    return `${this.room(seat.code, 'events')}?token=${encodeURIComponent(seat.token)}`;
  }

  private act(seat: RoomSeat, action: string): Promise<void> {
    return this.noContent(
      this.http.post(this.room(seat.code, action), { token: seat.token }, { responseType: 'text' }),
    );
  }

  private async noContent(request: Observable<unknown>): Promise<void> {
    await firstValueFrom(request);
  }
}
