package main

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lxn/win"
	"gocv.io/x/gocv"
)

//go:embed assets/*
var assets embed.FS

func scale_pos(p image.Point) image.Point {
	return image.Point{int(float64(p.X) / factor), int(float64(p.Y) / factor)}
}

var factor float64

func main() {
	var e emulator

	scaley := 960

	dronesizemin := 80
	dronesizemax := 400
	dronesizebrownmin := 25
	dronesizebrownmax := 300

	//
	thresholds := map[string]float32{
		"default":                   0.04,
		"ad_offer_watch_button":     0.055,
		"max_chicken_running_bonus": 0.06,
	}

	debug := true
	show_bad_detections := float32(0.08)

	templates := make(map[string]*template)

	err := e.Open()
	if err != nil {
		panic(err)
	}

	var window *gocv.Window
	if debug {
		window = gocv.NewWindow("Debug Window")
	}

	// Load templates
	detector := gocv.NewORB()
	fs.WalkDir(assets, ".", func(path string, file fs.DirEntry, err error) error {
		if strings.HasSuffix(file.Name(), ".png") {
			imagedata, err := assets.Open(path)
			if err != nil {
				panic(err)
			}
			loadedimage, err := png.Decode(imagedata)
			if err != nil {
				panic(err)
			}

			mat, _ := gocv.ImageToMatRGB(loadedimage)

			var alphaimage *image.Gray
			if loadedimage.ColorModel() == color.NRGBAModel {
				alphaimage = image.NewGray(loadedimage.Bounds())
				for x := 0; x < loadedimage.Bounds().Dx(); x++ {
					for y := 0; y < loadedimage.Bounds().Dy(); y++ {
						_, _, _, a := loadedimage.At(x, y).RGBA()
						av := uint8(a)
						alphaimage.Set(x, y, color.RGBA{av, av, av, av})
					}
				}
			}

			mask := gocv.NewMat()
			if alphaimage != nil {
				tmask, _ := gocv.ImageGrayToMatGray(alphaimage)
				tmask.ConvertTo(&mask, gocv.MatTypeCV32FC3)
				tmask.Close()
			}

			kps, m := detector.DetectAndCompute(mat, mask)
			m.Close()

			basename, height, found := strings.Cut(strings.TrimSuffix(file.Name(), ".png"), ".")
			if found {
				h, _ := strconv.ParseInt(height, 10, 64)
				factor := float64(scaley) / float64(h)
				oldx, oldy := mat.Cols(), mat.Rows()
				gocv.Resize(mat, &mat, image.Point{}, factor, factor, gocv.InterpolationLanczos4)
				fmt.Printf("Resized %s from %d,%d to %d,%d\n", basename, oldx, oldy, mat.Cols(), mat.Rows())
				if mask.Cols() > 0 {
					gocv.Resize(mask, &mask, image.Point{}, factor, factor, gocv.InterpolationLanczos4)
				}
			}

			threshold, found := thresholds[basename]
			if !found {
				threshold = thresholds["default"]
			}

			templates[basename] = &template{
				threshold: threshold,
				mat:       mat,
				mask:      mask,
				keypoints: kps,
			}
		}
		return nil
	})

	var resultlock sync.Mutex
	var lastimagetime time.Time
	var lastimage gocv.Mat

	var lastdronetakedowns []takedowninfo
	var lastresultstime time.Time
	var lastresults []result

	var lastdronetime = time.Now()
	var lastoktime = time.Now()

	running := true
	shoot_drones := true

	kernel := gocv.Ones(5, 5, gocv.MatTypeCV8U)

	// high speed screen capture, resize
	go func() {
		for running {
			if time.Since(lastimagetime) < time.Millisecond*20 { // 50Hz
				time.Sleep(time.Millisecond)
				continue
			}

			capture, err := e.Capture()
			if err != nil {
				// log.Warn().Error(err)
				running = false
				continue
			}

			screenmat, err := gocv.ImageToMatRGB(capture)
			if err != nil {
				panic(err)
			}

			factor := float64(scaley) / float64(screenmat.Rows())
			gocv.Resize(screenmat, &screenmat, image.Point{}, factor, factor, gocv.InterpolationLanczos4)

			resultlock.Lock()
			oldimage := lastimage
			lastimage = screenmat
			oldimage.Close()
			lastimagetime = time.Now()
			resultlock.Unlock()
		}
	}()

	// Drone detection
	go func() {
		var lastdroneprocessed time.Time
	runningloop:
		for running {
			if shoot_drones && !e.IsForeground() && !lastdroneprocessed.Equal(lastimagetime) {
				lastdroneprocessed = lastimagetime

				resultlock.Lock()
				dronedetectmat := lastimage.Clone()
				dronetakedowns := make([]takedowninfo, len(lastdronetakedowns))
				copy(dronetakedowns, lastdronetakedowns)
				resultlock.Unlock()

				if len(dronetakedowns) > 100 {
					var newdronetakedowns []takedowninfo
					for _, ti := range dronetakedowns {
						if time.Since(ti.timeout) < 0 {
							newdronetakedowns = append(newdronetakedowns, ti)
						}
					}
					dronetakedowns = newdronetakedowns
				}

				// drone detection - it's something black and brown
				blackdetect := gocv.NewMat()
				gocv.InRangeWithScalar(dronedetectmat, gocv.Scalar{30, 30, 30, 0}, gocv.Scalar{50, 50, 50, 0}, &blackdetect)
				dilatedblack := gocv.NewMat()
				gocv.Dilate(blackdetect, &dilatedblack, kernel)
				blackdetect.Close()
				blackcontours := gocv.FindContours(dilatedblack, gocv.RetrievalExternal, gocv.ChainApproxSimple)
				dilatedblack.Close()

				browndetect := gocv.NewMat()
				gocv.InRangeWithScalar(dronedetectmat, gocv.Scalar{70, 142, 183, 0}, gocv.Scalar{90, 162, 203, 0}, &browndetect)
				dilatedbrown := gocv.NewMat()
				gocv.Dilate(browndetect, &dilatedbrown, kernel)
				browndetect.Close()
				browncontours := gocv.FindContours(dilatedbrown, gocv.RetrievalExternal, gocv.ChainApproxSimple)
				dilatedbrown.Close()

				var blackrects, brownrects []image.Rectangle

				for _, contour := range blackcontours.ToPoints() {
					pv := gocv.NewPointVectorFromPoints(contour)
					bb := gocv.BoundingRect(pv)
					area := bb.Dx() * bb.Dy()

					if area > dronesizemin &&
						area < dronesizemax &&
						// (bb.Min.X > dronedetectmat.Cols()*25/100 ||
						// 	bb.Min.Y > dronedetectmat.Rows()*33/100) &&
						bb.Min.Y > dronedetectmat.Rows()*11/100 &&
						bb.Max.Y < dronedetectmat.Rows()*88/100 {
						blackrects = append(blackrects, bb)
					}

					pv.Close()
				}
				blackcontours.Close()

				for _, contour := range browncontours.ToPoints() {
					pv := gocv.NewPointVectorFromPoints(contour)
					bb := gocv.BoundingRect(pv)
					area := bb.Dx() * bb.Dy()

					if area > dronesizebrownmin &&
						area < dronesizebrownmax &&
						// (bb.Min.X > dronedetectmat.Cols()*25/100 ||
						// 	bb.Min.Y > dronedetectmat.Rows()*33/100) &&
						bb.Min.Y > dronedetectmat.Rows()*11/100 &&
						bb.Max.Y < dronedetectmat.Rows()*88/100 {
						brownrects = append(brownrects, bb)
					}

					pv.Close()
				}
				browncontours.Close()

				for _, blackrect := range blackrects {
					for _, brownrect := range brownrects {
						if blackrect.Overlaps(brownrect) {
							dronearea := blackrect.Intersect(brownrect)
							drone := image.Point{
								X: dronearea.Min.X + dronearea.Dx()/2,
								Y: dronearea.Min.Y + dronearea.Dy()/2,
							}

							blackarea := blackrect.Dx() * blackrect.Dy()
							brownarea := brownrect.Dx() * brownrect.Dy()

							fmt.Printf("Drone detected at %v, black area %v, brown area %v\n", drone, blackarea, brownarea)

							if true || blackarea*2 > brownarea*3 {
								closestindex := -1
								closestdist := 10000

								// Find cached info on the closest drone currently on screen
								for i, ti := range dronetakedowns {
									if time.Since(ti.timeout) > 0 {
										continue
									}

									if closestindex == -1 || distance(drone, ti.positions[len(ti.positions)-1]) < closestdist {
										closestindex = i
										closestdist = distance(drone, ti.positions[len(ti.positions)-1])
									}
								}

								// This is a new drone or it moved too much to be the same drone, add it and wait for it to be seen again to get vectors
								if closestindex == -1 || closestdist > dronedetectmat.Cols()/10 {
									dronetakedowns = append(dronetakedowns, takedowninfo{
										timeout:         time.Now().Add(time.Second),
										initialposition: drone,
										positions:       []image.Point{drone},
										seencount:       1,
									})
									resultlock.Lock()
									lastdronetime = time.Now()
									lastdronetakedowns = dronetakedowns
									resultlock.Unlock()

									continue
								}

								// Not new, so add new info to it
								dronetakedowns[closestindex].timeout = time.Now().Add(time.Second)
								dronetakedowns[closestindex].positions = append(dronetakedowns[closestindex].positions, drone)
								dronetakedowns[closestindex].seencount++

								resultlock.Lock()
								lastdronetime = time.Now()
								lastdronetakedowns = dronetakedowns
								resultlock.Unlock()

								if distance(drone, dronetakedowns[closestindex].positions[0]) < dronedetectmat.Cols()/10 {
									// Non-moving false positive
									continue
								}

								// Should we zap it?
								if len(dronetakedowns[closestindex].zaps) > 0 &&
									distance(drone, dronetakedowns[closestindex].zaps[len(dronetakedowns[closestindex].zaps)-1]) < dronedetectmat.Cols()/6 {
									// Nope
									continue
								}

								// Light it up
								fmt.Printf("Shooting down drone at %v, %v\n", drone.X, drone.Y)

								drone_pos_scaled := scale_pos(drone)

								e.MouseDown(drone_pos_scaled)

								for i := 0; i <= 5; i++ {
									time.Sleep(time.Millisecond * 3)
									e.MouseDrag(drone_pos_scaled.Add(image.Pt(0, i*3)))
								}
								for i := 5; i >= 0; i-- {
									time.Sleep(time.Millisecond * 3)
									e.MouseDrag(drone_pos_scaled.Add(image.Pt(0, i*3)))
								}
								e.MouseUp(drone_pos_scaled)

								dronetakedowns[closestindex].zaps = append(dronetakedowns[closestindex].zaps, drone)

								resultlock.Lock()
								lastdronetime = time.Now()
								lastdronetakedowns = dronetakedowns
								resultlock.Unlock()

								dronedetectmat.Close()
								continue runningloop
							}
						}
					}
				}

				dronedetectmat.Close()
			}
			time.Sleep(time.Millisecond * 5)
		}
	}()

	var ad_started time.Time
	var watching_ad bool

	// Image detection
	go func() {

		for running {
			if !e.IsForeground() && !lastimagetime.IsZero() && lastimagetime.After(lastresultstime) && time.Since(lastresultstime) > time.Millisecond*1000 {
				resultlock.Lock()
				screenmat := lastimage.Clone()
				resultlock.Unlock()

				results := make([]result, len(templates))

				var wg sync.WaitGroup

				wg.Add(len(templates))

				var i int
				for name, t := range templates {
					go func(name string, t *template, i int) {
						resultmat := gocv.NewMat()
						gocv.MatchTemplate(screenmat, t.mat, &resultmat, gocv.TmSqdiffNormed, t.mask)
						confidence, _, loc, _ := gocv.MinMaxLoc(resultmat)

						middle := loc.Add(image.Point{t.mat.Cols() / 2, t.mat.Rows() / 2})
						results[i] = result{
							name:       name,
							confidence: confidence,
							threshold:  t.threshold,
							location:   middle,
							rect:       image.Rect(loc.X, loc.Y, loc.X+t.mat.Cols(), loc.Y+t.mat.Rows()),
						}
						resultmat.Close()
						wg.Done()
					}(name, t, i)
					i++
				}

				wg.Wait()

				screenmat.Close()

				resultlock.Lock()
				lastresults = results
				lastresultstime = time.Now()
				resultlock.Unlock()
			}
		}
	}()

	go func() {
		var lastactiontime time.Time
		var last_double_video_boosts_open time.Time
		var last_ad_icon, last_package_icon image.Point
		var lastblurtime time.Time

		for running {
			if !e.IsForeground() && !lastactiontime.Equal(lastresultstime) {

				resultlock.Lock()
				screen := lastimage.Clone()
				results := make([]result, len(lastresults))
				copy(results, lastresults)
				// image_max_y := lastimage.Rows()
				resultlock.Unlock()

				// Local screen to use
				image_max_x := screen.Cols()

				var silo, hatchgreen,
					ok_button,
					close_button,
					watch_ad_boosts_button,
					watch_ad_round_offer_round_icon_button,
					boost_button,
					chickenbutton,
					red_dont_watch_ad_button,
					green_watch_ad_button,
					package_icon image.Point

				var max_chickens,
					ad_offer_accept,
					ad_offer_reject,
					boost_active_soul_mirror,
					video_double_indicator bool

				stillprocessing := true
				for _, res := range results {
					if res.confidence < res.threshold && stillprocessing {
						switch res.name {
						case "silo":
							silo = res.location
						case "daily_reward":
							// ignore
						case "boosts_button":
							boost_button = res.location
						case "video_double_indicator":
							video_double_indicator = true
						case "boost_active_soul_mirror":
							boost_active_soul_mirror = true
						case "boosts_watch_ad":
							watch_ad_boosts_button = res.location
						case "watch_ad":
							watch_ad_round_offer_round_icon_button = res.location
						case "ad_offer_boost", "ad_offer_eggs", "ad_offer_box_of_eggs", "ad_offer_crate_of_eggs", "ad_offer_chicken_box", "ad_offer_large_chicken_box":
							ad_offer_accept = true
						case "ad_offer_money", "ad_offer_a_ton_of_cash":
							ad_offer_reject = true
						case "ad_offer_no_thanks_button":
							red_dont_watch_ad_button = res.location
						case "ad_offer_watch_button":
							green_watch_ad_button = res.location
						case "blue_close_button",
							"green_close_button",
							"purple_close_button",
							"red_close_button":
							close_button = res.location
						case "lightblue_ok_button",
							"blue_ok_button",
							"pink_ok_button",
							"purple_ok_button":
							ok_button = res.location
							/*					case "green_research_button":
												fmt.Println("Auto researching 10x")
												e.Click(res.location.X, res.location.Y, 10)*/
						case "drone_1", "drone_2":
							// fmt.Println("Drone detected")
							// e.Click(middle.X, middle.Y, 1)
						case "launchicon":
							fmt.Println("Launching app")
							e.Click(scale_pos(res.location), 1)
							time.Sleep(time.Millisecond * 4000)
							shoot_drones = true
							stillprocessing = false
						case "mission_returned":
							fmt.Println("Mission returned")
							e.Click(scale_pos(res.location), 1)
							time.Sleep(time.Millisecond * 2000)
							stillprocessing = false
						case "collect_and_refill_silos_button",
							"collect_button",
							"purple_collect_button":
							fmt.Println("Collect button")
							e.Click(scale_pos(res.location), 1)
							time.Sleep(time.Millisecond * 500)
							stillprocessing = false
						case "max_chicken_running_bonus":
							max_chickens = true
						case "chickenbutton":
							chickenbutton = res.location
							shoot_drones = true
							lastoktime = time.Now()
						case "hatch_green":
							hatchgreen = res.location
						// case "daily_reward":
						// 	fmt.Printf("Daily reward at %v, %v\n", res.location.X, res.location.Y)
						// 	e.Click(res.location.X, res.location.Y, 1)
						// 	break templateloop
						case "package":
							package_icon = res.location
						default:
							fmt.Printf("Unknown detector %s\n", res.name)
						}
					}
				}

				// Blur detection
				if !watching_ad && time.Duration(boost_button.X) > 0 && time.Since(lastblurtime) > time.Second*15 {
					greymat := gocv.NewMat()
					gocv.CvtColor(screen, &greymat, gocv.ColorRGBToGray)
					lap := gocv.NewMat()
					gocv.Laplacian(greymat, &lap, gocv.MatTypeCV8U, 3, 1, 0, gocv.BorderDefault)
					dst := gocv.NewMat()
					dstStdDev := gocv.NewMat()
					gocv.MeanStdDev(lap, &dst, &dstStdDev)
					deviation := dstStdDev.Mean().Val1 * dstStdDev.Mean().Val1
					fmt.Printf("Deviation: %v\n", deviation)
					lastblurtime = time.Now()
					if deviation < 1000 {
						fmt.Println("Blurry screen detected, fixing")
						e.Click(scale_pos(boost_button), 1)
					}
					greymat.Close()
					lap.Close()
					dst.Close()
					dstStdDev.Close()
				}

				// take action
				if watching_ad {
					// Figure out if we're done watching ad
					if chickenbutton.X != 0 {
						fmt.Println("Ad complete")
						lastdronetime = time.Now()
						lastoktime = time.Now()
						watching_ad = false
						shoot_drones = true
					} else if time.Since(ad_started) > time.Second*45 {
						fmt.Println("Ad timed out/finished")
						e.SendKey(win.VK_HOME, 1)

						watching_ad = false
						shoot_drones = true
						lastdronetime = time.Now()
						lastoktime = time.Now()
					}
					time.Sleep(time.Second)
				} else if time.Since(lastoktime) > time.Second*60 || time.Since(lastdronetime) > time.Second*120 {
					fmt.Println("Crash detected, restarting app")

					fmt.Println("Clearning app task list")
					e.SendKey(win.VK_END, 1)
					for i := 0; i < 3; i++ {
						time.Sleep(time.Millisecond * 300)
						e.SendKey(win.VK_DELETE, 1)
						time.Sleep(time.Millisecond * 300)
						e.SendKey(win.VK_ESCAPE, 1)
						time.Sleep(time.Millisecond * 300)
					}
					e.SendKey(win.VK_HOME, 1)

					lastdronetime = time.Now()
					lastoktime = time.Now()
				} else if !boost_active_soul_mirror && green_watch_ad_button.X > 0 && red_dont_watch_ad_button.X > 0 {
					if ad_offer_reject {
						fmt.Println("Rejecting advertisement")
						e.Click(scale_pos(red_dont_watch_ad_button), 1)
						time.Sleep(time.Millisecond * 500)
					} else {
						if ad_offer_accept {
							fmt.Println("Accepting to watch advertisement")
						} else {
							fmt.Println("Unknown advertisement type, save screenshot ??? ... accepting it though")
							time.Sleep(time.Second * 5)
						}

						e.Click(scale_pos(green_watch_ad_button), 1)
						shoot_drones = false
						watching_ad = true
						ad_started = time.Now()
						time.Sleep(time.Second * 3)
					}
					lastoktime = time.Now()
				} else if !boost_active_soul_mirror && watch_ad_boosts_button.X > 0 {
					// Accept 2x boost from dialog
					fmt.Printf("Watching ad for boosts at %v, %v\n", watch_ad_boosts_button.X, watch_ad_boosts_button.Y)
					e.Click(scale_pos(watch_ad_boosts_button), 1)
					shoot_drones = false
					watching_ad = true
					ad_started = time.Now()
					lastoktime = time.Now()
					time.Sleep(time.Second * 3)
				} else if ok_button.X > 0 {
					fmt.Printf("Acknowleging dialog at %v, %v\n", ok_button.X, ok_button.Y)
					e.Click(scale_pos(ok_button), 1)
					time.Sleep(time.Millisecond * 1000)
					lastoktime = time.Now()
				} else if close_button.X > 0 {
					fmt.Printf("Closing dialog at %v, %v\n", close_button.X, close_button.Y)
					e.Click(scale_pos(close_button), 1)
					time.Sleep(time.Millisecond * 1000)
					lastoktime = time.Now()
				} else if !boost_active_soul_mirror && !video_double_indicator && boost_button.X > 0 && time.Since(last_double_video_boosts_open) > time.Minute*15 {
					fmt.Println("Opening boosts dialogue to get double video")
					last_double_video_boosts_open = time.Now()
					e.Click(scale_pos(boost_button), 1)
					time.Sleep(time.Millisecond * 1000) // Wait for dialog to settle
				} else if !boost_active_soul_mirror && watch_ad_round_offer_round_icon_button.X > image_max_x*7/10 && green_watch_ad_button.X == 0 {
					if last_ad_icon != watch_ad_round_offer_round_icon_button {
						// Needs to settle
						last_ad_icon = watch_ad_round_offer_round_icon_button
					} else {
						// Open dialog to possibly watch ad
						fmt.Println("Checking out ad offer")
						e.Click(scale_pos(watch_ad_round_offer_round_icon_button), 1)
						last_ad_icon = image.ZP
					}
				} else if package_icon.X > 0 {
					if last_package_icon != package_icon {
						// Needs to settle down
						last_package_icon = package_icon
					} else {
						fmt.Printf("Grabbing package at %v, %v\n", package_icon.X, package_icon.Y)
						e.Click(scale_pos(package_icon), 1)
						last_package_icon = image.ZP
					}
				} else if !max_chickens && hatchgreen.X > 0 && chickenbutton.X > 0 {
					fmt.Println("Hatching a lot of chickens")
					e.Click(scale_pos(chickenbutton), 250)
				} else if silo.X == 0 && chickenbutton.X > 0 {
					// Move screen so silo is visible
					fmt.Println("Moving screen to silo")
					middle := scale_pos(image.Pt(screen.Cols()/2, screen.Rows()/2))
					e.MouseDown(middle)
					for i := 0; i < 10; i++ {
						time.Sleep(time.Millisecond * 3)
						e.MouseDrag(middle.Add(image.Pt(i*3, i*3)))
					}
					e.MouseUp(middle.Add(image.Pt(30, 30)))
				}

				screen.Close()

				resultlock.Lock()
				lastactiontime = lastresultstime
				resultlock.Unlock()
			} else {
				time.Sleep(time.Millisecond * 100)
			}
		}
		fmt.Println("Analyzer main routine ending")
	}()

	var lastdebugimagetime time.Time
	var last_rect image.Rectangle
	for running {
		r, err := e.Rect()
		if err != nil {
			panic(err)
		}
		if r.Dx() > r.Dy() {
			// Landscape, so we need to rotate
			fmt.Println("Rotating screen to landscape")
			e.SendKey(win.VK_NEXT, 1) // PgDn
			time.Sleep(time.Millisecond * 2500)
			continue
		}

		if r != last_rect {
			fmt.Printf("Window found with size %v x %v\n", r.Dx(), r.Dy())
			factor = float64(scaley) / float64(r.Dy())
			if window != nil {
				window.ResizeWindow(r.Dx(), r.Dy())
			}
			last_rect = r
		}

		if window != nil && !lastdebugimagetime.Equal(lastimagetime) {
			lastdebugimagetime = lastimagetime

			resultlock.Lock()
			debugresults := make([]result, len(lastresults))
			copy(debugresults, lastresults)
			droneresults := make([]takedowninfo, len(lastdronetakedowns))
			copy(droneresults, lastdronetakedowns)
			debugmat := lastimage.Clone()
			resultlock.Unlock()

			// Resize debug window if needed

			// Draw debug window
			for _, result := range debugresults {
				var col color.RGBA
				var show bool
				if result.confidence < result.threshold {
					col = color.RGBA{128, 255, 128, 0}
					show = true
				} else if result.confidence < show_bad_detections {
					col = color.RGBA{255, 128, 128, 0}
					show = true
				}
				if show {
					gocv.Rectangle(&debugmat, result.rect, col, 2)
					gocv.PutText(&debugmat, fmt.Sprintf("%.2f %v", result.confidence, result.name), result.rect.Min.Add(image.Pt(4, 12)), gocv.FontHersheyPlain, 1, col, 2)
				}
			}

			for _, ti := range droneresults {
				for _, pos := range ti.positions {
					gocv.Circle(&debugmat, pos, 5, color.RGBA{0, 0, 255, 0}, -1)
				}
				for _, pos := range ti.zaps {
					gocv.Circle(&debugmat, pos, 5, color.RGBA{255, 0, 0, 0}, -1)
				}
			}

			window.IMShow(debugmat)
			if window.WaitKey(5) == 27 {
				// signal stop
				running = false
			}
			debugmat.Close()

		}

		time.Sleep(time.Millisecond * 25)
	}
}
